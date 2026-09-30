import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobDetailModel {
    private(set) var details: JobDetails?
    private(set) var loadError: String?
    private(set) var isAddingToPipeline = false
    var actionError: String?
    /// Where a "Read facts now" run stands, while one is going.
    private(set) var factsReadState: JobFactsReadState = .notQueued
    private(set) var isReadingFacts = false
    var factsError: String?

    func load(_ jobID: UUID, with client: HubClient) async {
        do {
            details = try await client.get("v1/jobs/\(jobID.uuidString)", as: JobDetails.self)
            loadError = nil
        } catch {
            details = nil
            loadError = String(describing: error)
        }
    }

    /// Adds the job to the pipeline, then shows its phase from a fresh read.
    func addToPipeline(_ jobID: UUID, with client: HubClient) async {
        isAddingToPipeline = true
        defer { isAddingToPipeline = false }
        do {
            _ = try await client.send("POST", "v1/applications", body: AddApplicationRequest(jobID: jobID), as: ApplicationResponse.self)
            await load(jobID, with: client)
        } catch {
            actionError = String(describing: error)
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
    @State private var model = JobDetailModel()

    var body: some View {
        Group {
            if let details = model.details, details.job.id == jobID {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        header(details)
                        actions(details)
                        if let connections = details.connections, !connections.isEmpty {
                            section("People you know at \(details.companyName ?? "this company")") {
                                ConnectionList(connections: connections)
                            }
                        }
                        fitChecks(details.fit)
                        boardFacts(details.job)
                        readFacts(details.facts)
                        if let description = details.job.description, !description.isEmpty {
                            section("Posting") {
                                Text(description).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
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
        .task(id: jobID) {
            await model.load(jobID, with: client)
            if let details = model.details, details.unseenUpdates > 0 {
                await unseen.markSeen(UpdateSelection(jobID: jobID), with: client)
            }
        }
        .alert("Could not add to the pipeline", isPresented: Binding(get: { model.actionError != nil }, set: { if !$0 { model.actionError = nil } })) {
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
        }
    }

    private func actions(_ details: JobDetails) -> some View {
        HStack {
            if let url = URL(string: details.job.url) {
                Button("Open posting", systemImage: "safari") { NSWorkspace.shared.open(url) }
            }
            if let phase = details.phase {
                Label("In \(phase.name)", systemImage: "rectangle.split.3x1").foregroundStyle(.secondary)
            } else {
                Button("Add to pipeline", systemImage: "plus") { Task { await model.addToPipeline(jobID, with: client) } }
                    .disabled(model.isAddingToPipeline)
                if model.isAddingToPipeline {
                    ProgressView().controlSize(.small)
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
