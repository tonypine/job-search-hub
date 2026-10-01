import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobDetailModel {
    private(set) var details: JobDetails?
    private(set) var loadError: String?
    var actionError: String?
    /// Where a "Read facts now" run stands, while one is going.
    private(set) var factsReadState: JobFactsReadState = .notQueued
    private(set) var isReadingFacts = false
    var factsError: String?
    private(set) var isWritingFullBrief = false

    func load(_ jobID: UUID, with client: HubClient) async {
        do {
            details = try await client.get("v1/jobs/\(jobID.uuidString)", as: JobDetails.self)
            loadError = nil
        } catch {
            details = nil
            loadError = String(describing: error)
        }
    }
}

extension JobDetailModel {
    /// Asks the hub to read the job's facts ahead of background work, follows
    /// the run through the model queue, then shows the fresh facts.
    func readFactsNow(_ jobID: UUID, with client: HubClient) async {
        isReadingFacts = true
        factsReadState = .queued
        defer {
            isReadingFacts = false
            factsReadState = .notQueued
        }
        do {
            _ = try await client.send("POST", "v1/jobs/\(jobID.uuidString)/facts/read", body: EmptyBody(), as: FactsReadResponse.self)
            let deadline = Date.now.addingTimeInterval(20 * 60)
            while Date.now < deadline {
                try await Task.sleep(for: .seconds(2))
                factsReadState = try await client.get("v1/model-work", as: ModelWork.self).getFactsReadState(for: jobID)
                if factsReadState == .notQueued { break }
            }
            await load(jobID, with: client)
        } catch is CancellationError {
        } catch {
            factsError = String(describing: error)
        }
    }
}

private struct FactsReadResponse: Decodable {
    var queued: Bool
}

extension JobDetailModel {
    /// Records the decision, then shows the job as it now stands.
    func decide(_ jobID: UUID, _ decision: JobDecisionKind, reason: String = "", through decisions: JobDecisions, with client: HubClient) async {
        do {
            _ = try await decisions.decide(jobID, decision, reason: reason, with: client)
            await load(jobID, with: client)
        } catch {
            actionError = String(describing: error)
        }
    }

    /// Asks Claude for the job's full brief, then follows the job until the
    /// full brief replaces the pre-brief, for up to three minutes.
    func writeFullBrief(_ jobID: UUID, with client: HubClient) async {
        isWritingFullBrief = true
        defer { isWritingFullBrief = false }
        let writtenBefore = details?.brief?.isFull == true ? details?.brief?.writtenAt : nil
        do {
            try await client.writeFullBrief(jobID)
            let deadline = Date.now.addingTimeInterval(3 * 60)
            while Date.now < deadline {
                try await Task.sleep(for: .seconds(3))
                await load(jobID, with: client)
                if let brief = details?.brief, brief.isFull, brief.writtenAt != writtenBefore { return }
            }
            actionError = "Claude hasn't written the brief yet; it shows here once it's saved."
        } catch is CancellationError {
        } catch {
            actionError = String(describing: error)
        }
    }
}

/// One job's details, read from the hub: what the board publishes, the facts
/// read from the posting, and the posting itself.
/// Which side of a job or company panel shows: what the hub knows, or its
/// Claude session.
enum PanelSide: String, CaseIterable, Identifiable {
    case details = "Details"
    case session = "Session"

    var id: String { rawValue }
}

/// A job's panel: its details, or its Claude session.
struct JobPanel: View {
    let jobID: UUID
    let client: HubClient
    /// Opens on the Session side and starts or resumes the session there.
    var opensSession = false
    @State private var side: PanelSide = .details

    var body: some View {
        VStack(spacing: 0) {
            Picker("Show", selection: $side) {
                ForEach(PanelSide.allCases) { side in Text(side.rawValue).tag(side) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(8)
            switch side {
            case .details: JobDetailView(jobID: jobID, client: client)
            case .session: ClaudeSessionPane(subject: .job(jobID), client: client, startsOnAppear: opensSession)
            }
        }
        .onAppear { if opensSession { side = .session } }
    }
}

struct JobDetailView: View {
    let jobID: UUID
    let client: HubClient
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(JobDecisions.self) private var decisions
    @State private var model = JobDetailModel()
    @State private var isAskingForDismissal = false
    @State private var isPostingShown = false

    var body: some View {
        Group {
            if let details = model.details, details.job.id == jobID {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        header(details)
                        actions(details)
                        brief(details.brief)
                        if details.decision?.decision == .pursue || details.cvID != nil {
                            JobCVSection(jobID: jobID, details: details, client: client)
                        }
                        screenOutAnswers(details.screenOut ?? [])
                        if let connections = details.connections, !connections.isEmpty {
                            section("People you know at \(details.companyName ?? "this company")") {
                                ConnectionList(connections: connections)
                            }
                        }
                        fitChecks(details.fit)
                        boardFacts(details.job)
                        readFacts(details.facts)
                        if let description = details.job.description, !description.isEmpty {
                            // With a brief to decide from, the posting folds away.
                            DisclosureGroup(isExpanded: Binding(get: { isPostingShown || details.brief == nil }, set: { isPostingShown = $0 })) {
                                Text(description).textSelection(.enabled).fixedSize(horizontal: false, vertical: true).padding(.top, 4)
                            } label: {
                                Text("Posting").font(.headline)
                            }
                        }
                    }
                    .padding(20)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else if let loadError = model.loadError {
                ContentUnavailableView("Could not load the job", systemImage: "exclamationmark.triangle", description: Text(loadError))
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .onChange(of: decisions.revision) { Task { await model.load(jobID, with: client) } }
        .sheet(isPresented: $isAskingForDismissal) {
            DismissJobsSheet(jobCount: 1, actionName: "Skip") { reason in
                do {
                    _ = try await decisions.decide(jobID, .skip, reason: reason, with: client)
                    return nil
                } catch {
                    return String(describing: error)
                }
            }
        }
        .task(id: jobID) {
            await model.load(jobID, with: client)
            if let details = model.details, details.unseenUpdates > 0 {
                await unseen.markSeen(UpdateSelection(jobID: jobID), with: client)
            }
        }
        .alert("Could not update the job", isPresented: Binding(get: { model.actionError != nil }, set: { if !$0 { model.actionError = nil } })) {
            Button("OK") {}
        } message: {
            Text(model.actionError ?? "")
        }
        .alert("Could not read the facts", isPresented: Binding(get: { model.factsError != nil }, set: { if !$0 { model.factsError = nil } })) {
            Button("OK") {}
        } message: {
            Text(model.factsError ?? "")
        }
    }

    private func header(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(details.job.title).font(.title2.weight(.semibold)).textSelection(.enabled)
            Text([details.companyName, details.job.location].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "))
                .foregroundStyle(.secondary)
            if details.job.dismissedAt != nil {
                let reason = details.job.dismissalReason ?? ""
                Label(reason.isEmpty ? "Dismissed" : "Dismissed: \(reason)", systemImage: "eye.slash").foregroundStyle(.orange)
            }
        }
    }

    /// Open the posting, and decide: Pursue, Skip or Later, with the decision
    /// already made.
    private func actions(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                if let url = URL(string: details.job.url) {
                    Button("Open posting", systemImage: "safari") { NSWorkspace.shared.open(url) }
                }
                if let phase = details.phase {
                    Label("In \(phase.name)", systemImage: "rectangle.split.3x1").foregroundStyle(.secondary)
                } else {
                    Button("Pursue", systemImage: "arrow.up.forward.circle") {
                        Task { await model.decide(jobID, .pursue, through: decisions, with: client) }
                    }
                    .help("Put it on the pipeline")
                }
                if details.job.dismissedAt != nil {
                    Button("Restore", systemImage: "arrow.uturn.backward") {
                        Task {
                            do {
                                _ = try await decisions.restore([jobID], with: client)
                            } catch {
                                model.actionError = String(describing: error)
                            }
                        }
                    }
                } else {
                    Button("Skip…", systemImage: "eye.slash") { isAskingForDismissal = true }
                        .help("Dismiss it, with a reason")
                }
                if details.decision?.decision != .later && details.phase == nil && details.job.dismissedAt == nil {
                    Button("Later", systemImage: "clock") { Task { await model.decide(jobID, .later, through: decisions, with: client) } }
                        .help("Leave it for another day")
                }
            }
            if let decision = details.decision {
                Text(describeDecision(decision)).font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    private func describeDecision(_ decision: JobDecision) -> String {
        var text = "\(decision.decision.pastTense) \(decision.decidedAt.formatted(.relative(presentation: .named)))"
        if let reason = decision.reason, !reason.isEmpty {
            text += ": \(reason)"
        }
        return text
    }

    /// The brief the decision rests on: the match and why, then the strengths
    /// and weaknesses with the knowledge-base entries behind them.
    @ViewBuilder
    private func brief(_ brief: JobBrief?) -> some View {
        section("Brief") {
            if let brief {
                HStack(spacing: 8) {
                    MatchLabel(match: brief.match)
                    Text(brief.isFull ? "by Claude" : "by the local model").font(.caption).foregroundStyle(.secondary)
                    if brief.isStale {
                        Label("Your knowledge base changed since", systemImage: "clock.arrow.circlepath").font(.caption).foregroundStyle(.orange)
                    }
                }
                Text(brief.reason).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                briefPoints("Strengths", brief.strengths, in: brief, symbol: "plus.circle.fill", color: .green)
                briefPoints("Weaknesses", brief.weaknesses, in: brief, symbol: "minus.circle.fill", color: .orange)
            } else {
                Text("Not briefed yet. The local model briefs good and unclear jobs once their facts are read.").foregroundStyle(.secondary)
            }
            if brief?.isFull != true {
                HStack(spacing: 8) {
                    Button(model.isWritingFullBrief ? "Writing…" : "Write full brief", systemImage: "sparkles") {
                        Task { await model.writeFullBrief(jobID, with: client) }
                    }
                    .disabled(model.isWritingFullBrief)
                    .help("Have Claude write a fuller brief now")
                    if model.isWritingFullBrief {
                        ProgressView().controlSize(.small)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func briefPoints(_ title: String, _ points: [JobBriefPoint], in brief: JobBrief, symbol: String, color: Color) -> some View {
        if !points.isEmpty {
            Text(title).font(.subheadline.weight(.semibold))
            ForEach(Array(points.enumerated()), id: \.offset) { _, point in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(point.point).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                        let entries = brief.getEntries(of: point)
                        if !entries.isEmpty {
                            Text(entries.map(\.label).joined(separator: "; ")).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                } icon: {
                    Image(systemName: symbol).foregroundStyle(color)
                }
            }
        }
    }

    /// What could screen you out at once, answered from the posting's words.
    @ViewBuilder
    private func screenOutAnswers(_ answers: [ScreenOutAnswer]) -> some View {
        if !answers.isEmpty {
            section("Screen-out checks") {
                ForEach(answers) { answer in
                    Label {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(answer.name).fontWeight(.medium) + Text("  \(answer.answer)").foregroundStyle(.secondary)
                            if let evidence = answer.evidence {
                                Text("\u{201C}\(evidence)\u{201D}").font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                            }
                        }
                    } icon: {
                        if let verdict = answer.verdict {
                            Image(systemName: verdictSymbol(verdict)).foregroundStyle(verdictColor(verdict))
                        } else {
                            Image(systemName: "info.circle").foregroundStyle(.secondary)
                        }
                    }
                }
            }
        }
    }

    private func fitChecks(_ fit: JobFit) -> some View {
        section("Fit") {
            HStack(spacing: 6) {
                FitLabel(level: fit.level)
                Text("for your criteria").foregroundStyle(.secondary)
            }
            ForEach(fit.checks) { check in
                Label {
                    Text(check.name).fontWeight(.medium) + Text("  \(check.reason)").foregroundStyle(.secondary)
                } icon: {
                    Image(systemName: verdictSymbol(check.verdict)).foregroundStyle(verdictColor(check.verdict))
                }
            }
        }
    }

    private func verdictSymbol(_ verdict: FitVerdict) -> String {
        switch verdict {
        case .yes: "checkmark.circle.fill"
        case .no: "xmark.circle.fill"
        case .unclear: "questionmark.circle"
        }
    }

    private func verdictColor(_ verdict: FitVerdict) -> Color {
        switch verdict {
        case .yes: .green
        case .no: .red
        case .unclear: .orange
        }
    }

    private func boardFacts(_ job: Job) -> some View {
        section("From the board") {
            Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 6) {
                if let pay = job.pay {
                    factRow("Pay") {
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(Array(pay.ranges.enumerated()), id: \.offset) { _, range in
                                Text([range.label, range.format()].compactMap { $0 }.joined(separator: ": "))
                            }
                            if let summary = pay.summary {
                                Text(summary).foregroundStyle(.secondary)
                            }
                        }
                    }
                } else {
                    factRow("Pay") { Text("Not published").foregroundStyle(.secondary) }
                }
                textRow("Workplace", job.workplaceType)
                textRow("Employment", job.employmentType)
                textRow("Department", job.department)
                textRow("Also hiring in", job.otherLocations?.joined(separator: ", "))
                textRow("Published", job.publishedAt?.formatted(date: .abbreviated, time: .omitted))
                textRow("First seen", job.firstSeenAt.formatted(date: .abbreviated, time: .omitted))
            }
        }
    }

    @ViewBuilder
    private func readFacts(_ facts: LabelledJobFacts?) -> some View {
        if let facts {
            section("Read from the posting") {
                Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 6) {
                    ForEach(facts.entries) { entry in
                        factRow(entry.title, help: entry.description) {
                            VStack(alignment: .leading, spacing: 2) {
                                switch entry.display {
                                case let .text(text): Text(text).textSelection(.enabled)
                                case let .list(items): Text(items.joined(separator: ", ")).textSelection(.enabled)
                                case .notStated: Text("Not stated").foregroundStyle(.secondary)
                                }
                                if let evidence = entry.evidence {
                                    Text("\u{201C}\(evidence)\u{201D}")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                        .textSelection(.enabled)
                                }
                            }
                        }
                    }
                }
                Text("Read by \(facts.model) with prompt version \(facts.promptVersion), \(facts.extractedAt.formatted(date: .abbreviated, time: .shortened)).")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                readFactsNowRow
            }
        } else {
            section("Read from the posting") {
                Text("Not read yet. The hub reads new postings as they arrive, unless its model work is paused.")
                    .foregroundStyle(.secondary)
                readFactsNowRow
            }
        }
    }

    /// The "Read facts now" button, and where its run stands.
    private var readFactsNowRow: some View {
        HStack(spacing: 8) {
            Button(model.isReadingFacts ? "Reading…" : "Read facts now", systemImage: "arrow.clockwise") {
                Task { await model.readFactsNow(jobID, with: client) }
            }
            .disabled(model.isReadingFacts)
            switch model.factsReadState {
            case .queued:
                ProgressView().controlSize(.small)
                Text("Waiting for its turn").foregroundStyle(.secondary)
            case .running:
                ProgressView().controlSize(.small)
                Text("Reading now").foregroundStyle(.secondary)
            case .notQueued:
                EmptyView()
            }
        }
        .padding(.top, 4)
    }

    @ViewBuilder
    private func textRow(_ title: String, _ value: String?) -> some View {
        if let value, !value.isEmpty {
            factRow(title) { Text(value).textSelection(.enabled) }
        }
    }

    private func factRow<Value: View>(_ title: String, help: String? = nil, @ViewBuilder value: () -> Value) -> some View {
        GridRow {
            Text(title).foregroundStyle(.secondary).gridColumnAlignment(.trailing).help(help ?? "")
            value().frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func section<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.headline)
            content()
        }
    }
}

/// A brief's match as a colored word.
struct MatchLabel: View {
    let match: JobMatch

    var body: some View {
        Text(match.title)
            .font(.callout.weight(.semibold))
            .padding(.horizontal, 8)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }

    private var color: Color {
        switch match {
        case .strong: .green
        case .possible: .blue
        case .stretch: .orange
        case .mismatch: .secondary
        }
    }
}
