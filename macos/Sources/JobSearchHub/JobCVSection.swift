import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobCVModel {
    private(set) var cv: CV?
    private(set) var base: CV?
    private(set) var screen: CVScreen?
    private(set) var isLoading = false
    private(set) var isSaving = false
    private(set) var isPrinting = false
    private(set) var isDrafting = false
    private(set) var notice: String?
    var error: String?
    /// The words being edited: the headline, summary and each role's bullets.
    var label = ""
    var summary = ""
    var bullets: [[String]] = []

    var comparison: CVComparison? {
        guard let cv, let base else { return nil }
        return CVComparison(tailored: cv, base: base)
    }

    var hasEdits: Bool {
        guard let cv else { return false }
        return label != cv.content.basics.label || summary != cv.content.basics.summary || bullets != cv.content.work.map(\.highlights)
    }

    func load(_ jobID: UUID, with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            base = try await client.getBaseCV()
            show(try await client.getJobCV(jobID))
            screen = try? await client.getCVScreen(jobID)
            error = nil
        } catch HubError.notFound {
            cv = nil
        } catch {
            self.error = String(describing: error)
        }
    }

    func saveEdits(_ jobID: UUID, with client: HubClient) async {
        guard let cv else { return }
        isSaving = true
        defer { isSaving = false }
        let roles = bullets.enumerated().map { workIndex, texts in
            CVRoleEdit(role: workIndex, bullets: texts.enumerated().map { highlightIndex, text in
                CVBulletEdit(text: text, source: cv.citations["w\(workIndex)h\(highlightIndex)"] ?? "")
            })
        }
        do {
            show(try await client.saveCVEdit(jobID, CVEdit(label: label, summary: summary, roles: roles)))
            notice = "Saved"
        } catch {
            self.error = String(describing: error)
        }
    }

    func removeBullet(role: Int, at index: Int) {
        guard let cv, bullets.indices.contains(role), bullets[role].indices.contains(index) else { return }
        // Citations follow the bullets' places, so the ones after shift up.
        var citations = cv.citations
        for later in index..<(bullets[role].count - 1) {
            citations["w\(role)h\(later)"] = cv.citations["w\(role)h\(later + 1)"]
        }
        citations["w\(role)h\(bullets[role].count - 1)"] = nil
        bullets[role].remove(at: index)
        self.cv?.citations = citations
        self.cv?.content.work[role].highlights.remove(at: index)
    }

    /// Asks Claude for a new draft, then follows the job until it's saved,
    /// for up to three minutes.
    func draftAgain(_ jobID: UUID, with client: HubClient) async {
        isDrafting = true
        defer { isDrafting = false }
        let draftedBefore = cv?.id
        do {
            try await client.draftCV(jobID)
            let deadline = Date.now.addingTimeInterval(3 * 60)
            while Date.now < deadline {
                try await Task.sleep(for: .seconds(3))
                if let drafted = try? await client.getJobCV(jobID), drafted.id != draftedBefore || drafted.content != cv?.content {
                    show(drafted)
                    return
                }
            }
            error = "Claude hasn't finished the draft yet; it shows here once it's saved."
        } catch is CancellationError {
        } catch {
            self.error = String(describing: error)
        }
    }

    /// Has the hub print the CV to its PDF file, then opens it.
    func printPDF(with client: HubClient) async {
        guard let cv else { return }
        isPrinting = true
        defer { isPrinting = false }
        do {
            let printed = try await client.printCV(cv.id)
            self.cv = printed
            if let path = printed.pdfPath {
                notice = "Printed to \(path)"
                NSWorkspace.shared.open(URL(filePath: path))
            }
        } catch {
            self.error = String(describing: error)
        }
    }

    private func show(_ cv: CV) {
        self.cv = cv
        label = cv.content.basics.label
        summary = cv.content.basics.summary
        bullets = cv.content.work.map(\.highlights)
    }
}

/// A pursued job's tailored CV: its words against the base CV, editable, and
/// printed to PDF on the Mac.
struct JobCVSection: View {
    let jobID: UUID
    let details: JobDetails
    let client: HubClient
    @State private var model = JobCVModel()

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("CV").font(.headline)
                Spacer()
                if let file = model.cv?.printedFile {
                    Button("Open CV", systemImage: "doc.richtext") { NSWorkspace.shared.open(file) }
                        .help(file.path)
                    Button("Show in Finder", systemImage: "folder") { NSWorkspace.shared.activateFileViewerSelecting([file]) }
                } else if model.cv != nil {
                    Text("Printing within a few minutes").font(.caption).foregroundStyle(.secondary)
                }
            }
            if let comparison = model.comparison {
                editor(comparison)
            } else if model.isLoading {
                ProgressView().controlSize(.small)
            } else {
                Text(details.decision?.decision == .pursue || details.fit.level == .good
                     ? "Claude drafts and prints a CV for this job within a few minutes."
                     : "Pursue the job, or draft a CV now.").foregroundStyle(.secondary)
                draftButton
            }
            if let notice = model.notice {
                Text(notice).font(.caption).foregroundStyle(.secondary)
            }
            if let cv = model.cv {
                CVScreenView(screen: model.screen, cv: cv)
            }
            if let error = model.error {
                Text(error).font(.caption).foregroundStyle(.red)
            }
        }
        .task(id: details.cvID) { await model.load(jobID, with: client) }
    }

    private var draftButton: some View {
        HStack(spacing: 8) {
            Button(model.isDrafting ? "Drafting…" : (model.cv == nil ? "Draft CV" : "Draft again"), systemImage: "doc.text") {
                Task { await model.draftAgain(jobID, with: client) }
            }
            .disabled(model.isDrafting)
            if model.isDrafting {
                ProgressView().controlSize(.small)
            }
        }
    }

    private func editor(_ comparison: CVComparison) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            labelled("Headline", changed: comparison.isLabelChanged) {
                TextField("Headline", text: $model.label).accessibilityLabel("CV headline")
            }
            labelled("Summary", changed: comparison.isSummaryChanged) {
                TextEditor(text: $model.summary).frame(minHeight: 80).font(.body).accessibilityLabel("CV summary")
            }
            ForEach(comparison.roles) { role in
                if !role.bullets.isEmpty || !role.leftOut.isEmpty {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("\(role.position) · \(role.company)").font(.subheadline.weight(.semibold))
                        ForEach(Array(role.bullets.enumerated()), id: \.offset) { index, bullet in
                            HStack(alignment: .firstTextBaseline, spacing: 6) {
                                statusLabel(bullet.status)
                                TextField("Bullet", text: bulletBinding(role: role.index, index: index), axis: .vertical)
                                    .accessibilityLabel("Bullet \(role.index)-\(index)")
                                Button("Remove", systemImage: "minus.circle") { model.removeBullet(role: role.index, at: index) }
                                    .labelStyle(.iconOnly).buttonStyle(.borderless)
                            }
                        }
                        ForEach(role.leftOut, id: \.self) { text in
                            Text("Left out: \(text)").font(.caption).foregroundStyle(.tertiary)
                        }
                    }
                }
            }
            HStack(spacing: 8) {
                Button("Save edits") { Task { await model.saveEdits(jobID, with: client) } }
                    .disabled(!model.hasEdits || model.isSaving)
                Button(model.isPrinting ? "Printing…" : "Print to PDF", systemImage: "printer") {
                    Task { await model.printPDF(with: client) }
                }
                .disabled(model.isPrinting || model.hasEdits)
                .help(model.hasEdits ? "Save the edits first" : "Print the CV and keep the PDF")
                draftButton
            }
        }
    }

    private func labelled<Content: View>(_ title: String, changed: Bool, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 6) {
                Text(title).font(.caption.weight(.semibold))
                if changed {
                    Text("tailored").font(.caption2).foregroundStyle(.blue)
                }
            }
            content()
        }
    }

    @ViewBuilder
    private func statusLabel(_ status: CVBulletStatus) -> some View {
        switch status {
        case .kept: Text("kept").font(.caption2).foregroundStyle(.secondary).help("Word for word from your CV")
        case let .reworded(from): Text("reworded").font(.caption2).foregroundStyle(.blue).help("Your CV says: \(from)")
        case .fromEntry: Text("new").font(.caption2).foregroundStyle(.green).help("From a confirmed knowledge-base entry")
        }
    }

    private func bulletBinding(role: Int, index: Int) -> Binding<String> {
        Binding(
            get: { model.bullets.indices.contains(role) && model.bullets[role].indices.contains(index) ? model.bullets[role][index] : "" },
            set: { if model.bullets.indices.contains(role) && model.bullets[role].indices.contains(index) { model.bullets[role][index] = $0 } }
        )
    }

    /// "Acme - Senior Engineer", safe as a file name.
}

/// The recruiter screen of the tailored CV: the verdict, then each issue with
/// what the posting says and how the CV can answer it.
private struct CVScreenView: View {
    let screen: CVScreen?
    let cv: CV

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Recruiter screen").font(.subheadline.weight(.semibold))
            if let screen {
                Label(screen.screen.verdict.title, systemImage: verdictSymbol(screen.screen.verdict))
                    .foregroundStyle(verdictColor(screen.screen.verdict))
                Text(screen.screen.summary).foregroundStyle(.secondary)
                ForEach(Array(screen.screen.issues.enumerated()), id: \.offset) { _, issue in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(issue.issue).fontWeight(.medium)
                        if issue.postingSays != "not stated" {
                            Text("“\(issue.postingSays)”").font(.caption).italic().foregroundStyle(.secondary)
                        }
                        Text(issue.response).font(.callout)
                    }
                    .padding(.leading, 8)
                }
                if screen.isOutdated(for: cv) {
                    Text("The CV changed since; it's screened again within a few minutes.").font(.caption).foregroundStyle(.secondary)
                }
            } else {
                Text("The local model screens the CV as the job's recruiter would, within a few minutes of drafting it.")
                    .font(.callout).foregroundStyle(.secondary)
            }
        }
        .padding(.top, 4)
    }

    private func verdictSymbol(_ verdict: CVScreenVerdict) -> String {
        switch verdict {
        case .likelyPass: "checkmark.circle.fill"
        case .borderline: "questionmark.circle.fill"
        case .likelyReject: "xmark.circle.fill"
        }
    }

    private func verdictColor(_ verdict: CVScreenVerdict) -> Color {
        switch verdict {
        case .likelyPass: .green
        case .borderline: .orange
        case .likelyReject: .red
        }
    }
}
