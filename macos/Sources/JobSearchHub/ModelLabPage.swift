import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompareModel {
    private(set) var comparisons: [Comparison] = []
    private(set) var record: ComparisonRecord?
    var failure: HubFailure?
    private(set) var isSavingVerdict = false
    var selectedID: UUID?
    var postingIndex = 0

    var hasRunningComparison: Bool { comparisons.contains(where: \.isRunning) }

    /// Reads the list, and again every five seconds while a comparison in it
    /// runs, so each row's state stays current.
    func watchList(with client: HubClient) async {
        repeat {
            await loadList(with: client)
            if !hasRunningComparison { return }
            try? await Task.sleep(for: .seconds(5))
        } while !Task.isCancelled
    }

    func loadList(with client: HubClient) async {
        do {
            comparisons = try await client.getComparisons()
            failure = nil
            if selectedID == nil || !comparisons.contains(where: { $0.id == selectedID }) {
                selectedID = comparisons.first?.id
            }
        } catch {
            failure = HubFailure("Couldn't load the comparisons", error)
        }
    }

    /// Reads the selected comparison, and again every five seconds while it
    /// runs, so answers show up as they land.
    func watchSelected(with client: HubClient) async {
        guard let selectedID else {
            record = nil
            return
        }
        if record?.comparison.id != selectedID {
            record = nil
            postingIndex = 0
        }
        while !Task.isCancelled {
            do {
                let loaded = try await client.getComparison(selectedID)
                record = loaded
                failure = nil
                if !loaded.comparison.isRunning {
                    if let index = comparisons.firstIndex(where: { $0.id == loaded.comparison.id }) {
                        comparisons[index] = loaded.comparison
                    }
                    return
                }
            } catch {
                failure = HubFailure("Couldn't load the comparison", error)
                return
            }
            try? await Task.sleep(for: .seconds(5))
        }
    }

    /// Marks a stack's field on a posting right or wrong, replacing an
    /// earlier mark.
    func setVerdict(_ verdict: ComparisonVerdictKind, stackID: UUID, jobID: UUID, field: String, with client: HubClient) async {
        guard let comparisonID = record?.comparison.id else { return }
        isSavingVerdict = true
        defer { isSavingVerdict = false }
        do {
            record = try await client.saveComparisonVerdicts(
                [ComparisonVerdict(stackID: stackID, jobID: jobID, field: field, verdict: verdict)], comparisonID: comparisonID
            )
        } catch {
            failure = HubFailure("Couldn't save the verdict", error)
        }
    }

    func start(_ comparison: NewComparison, with client: HubClient) async throws {
        let started = try await client.startComparison(comparison)
        await loadList(with: client)
        selectedID = started.comparison.id
    }
}

/// Model comparisons: each one's agreement with its first stack and the
/// owner's verdicts, field by field, and its postings to judge side by side.
struct ModelLabPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @State private var model = CompareModel()
    @State private var isAddingComparison = false

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client)
                    .task(id: model.hasRunningComparison) { await model.watchList(with: client) }
                    .task(id: model.selectedID) { await model.watchSelected(with: client) }
                    .onChange(of: events.revision) { Task { await model.loadList(with: client) } }
                    .toolbar {
                        Menu("Add", systemImage: "plus") {
                            Button("Comparison…") { isAddingComparison = true }
                        }
                        .help("Add a comparison (⌘N)")
                    }
                    .focusedSceneValue(\.pageAdd, PageAddAction(title: "Add Comparison…") { isAddingComparison = true })
                    .sheet(isPresented: $isAddingComparison) {
                        NewComparisonSheet(client: client) { comparison in
                            try await model.start(comparison, with: client)
                        }
                    }
            }
        }
        .navigationTitle("Model lab")
    }

    private func content(_ client: HubClient) -> some View {
        HSplitView {
            List(model.comparisons, selection: $model.selectedID) { comparison in
                VStack(alignment: .leading, spacing: 2) {
                    Text(comparison.title).lineLimit(2)
                    Text(describeComparison(comparison)).font(.hubCaption).foregroundStyle(.secondary)
                }
                .tag(comparison.id)
            }
            .frame(minWidth: 220, idealWidth: 260, maxWidth: 360)
            Group {
                if let record = model.record {
                    ComparisonView(record: record, model: model, client: client)
                } else if model.comparisons.isEmpty {
                    ContentUnavailableView("No comparisons", systemImage: "square.split.2x1", description: Text("Start one to run models side by side."))
                } else {
                    ProgressView()
                }
            }
            .frame(minWidth: 520, maxWidth: .infinity, maxHeight: .infinity)
        }
        .overlay(alignment: .bottom) {
            if model.failure != nil {
                HubErrorView($model.failure)
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
    }

    private func describeComparison(_ comparison: Comparison) -> String {
        let state = comparison.isRunning ? "running" : comparison.createdAt.formatted(date: .abbreviated, time: .omitted)
        return "\(comparison.stacks.count) stacks · \(comparison.postingCountText) · \(state)"
    }
}

private enum ComparisonTab: String, CaseIterable, Identifiable {
    case summary = "Summary"
    case postings = "Postings"

    var id: String { rawValue }
}

/// One comparison: its summary grid, or its postings one at a time.
private struct ComparisonView: View {
    let record: ComparisonRecord
    let model: CompareModel
    let client: HubClient
    @State private var tab = ComparisonTab.summary

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(record.comparison.title).font(.hubEntity)
                    Text(describeProgress()).font(.hubSecondary).foregroundStyle(.secondary)
                }
                Spacer()
                Picker("View", selection: $tab) {
                    ForEach(ComparisonTab.allCases) { Text($0.rawValue).tag($0) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .fixedSize()
            }
            switch tab {
            case .summary: ComparisonSummaryGrid(record: record)
            case .postings: ComparisonPostingsView(record: record, model: model, client: client)
            }
        }
        .padding(Space.l)
    }

    private func describeProgress() -> String {
        let runStacks = record.comparison.stacks.filter { !$0.isImported }
        let expected = runStacks.count * record.comparison.jobIDs.count
        let runAnswers = record.answers.filter { answer in runStacks.contains { $0.id == answer.stackID } }.count
        if record.comparison.isRunning {
            return "Running · \(runAnswers) of \(expected) answers"
        }
        return "Done · \(record.comparison.postingCountText) · measured against \(record.comparison.stacks.first?.label ?? "the first stack")"
    }
}

/// Fields down, stacks across: each cell is the stack's agreement with the
/// first, and the owner's right and wrong marks.
private struct ComparisonSummaryGrid: View {
    let record: ComparisonRecord

    var body: some View {
        ScrollView {
            Grid(alignment: .leading, horizontalSpacing: Space.l, verticalSpacing: Space.s) {
                GridRow {
                    Text("Field").font(.hubSection)
                    ForEach(record.comparison.stacks) { stack in
                        VStack(alignment: .leading) {
                            Text(stack.label).font(.hubSection)
                            Text(describeAnswers(stack)).font(.hubCaption).foregroundStyle(.secondary)
                        }
                    }
                }
                Divider()
                ForEach(record.summary.fields, id: \.self) { field in
                    GridRow {
                        Text(ComparisonSummary.formatFieldTitle(field))
                        ForEach(record.comparison.stacks) { stack in
                            scoreCell(record.getScore(stackID: stack.id, field: field), isReference: stack.id == record.comparison.stacks.first?.id)
                        }
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.bottom)
        }
    }

    private func describeAnswers(_ stack: ComparisonStack) -> String {
        guard let summary = record.summary.stacks.first(where: { $0.stackID == stack.id }) else { return "" }
        return summary.failed == 0 ? "\(summary.answered) answered" : "\(summary.answered) answered · \(summary.failed) failed"
    }

    @ViewBuilder
    private func scoreCell(_ score: ComparisonFieldScore?, isReference: Bool) -> some View {
        HStack(spacing: Space.s) {
            Text(isReference ? "reference" : score?.agreementText ?? "—")
                .foregroundStyle(isReference ? .secondary : .primary)
                .monospacedDigit()
            if let score, score.right > 0 {
                ToneChip("\(score.right)", tone: ComparisonVerdictKind.right.tone, symbol: "checkmark")
                    .accessibilityLabel("\(score.right) right")
            }
            if let score, score.wrong > 0 {
                ToneChip("\(score.wrong)", tone: ComparisonVerdictKind.wrong.tone, symbol: "xmark")
                    .accessibilityLabel("\(score.wrong) wrong")
            }
        }
        .font(.hubSecondary)
    }
}

/// One posting at a time: each field with every stack's reading, its
/// evidence, and Right and Wrong to judge it.
private struct ComparisonPostingsView: View {
    let record: ComparisonRecord
    let model: CompareModel
    let client: HubClient
    @Environment(DetailsInspector.self) private var details
    @State private var showsOnlyDifferences = true

    var body: some View {
        if record.jobs.isEmpty {
            ContentUnavailableView("No postings", systemImage: "doc.text")
        } else {
            let index = min(model.postingIndex, record.jobs.count - 1)
            let job = record.jobs[index]
            VStack(alignment: .leading, spacing: Space.s) {
                HStack {
                    Button("Previous posting", systemImage: "chevron.left") { model.postingIndex = index - 1 }
                        .labelStyle(.iconOnly)
                        .disabled(index == 0)
                    Text("\(index + 1) of \(record.jobs.count)").monospacedDigit()
                    Button("Next posting", systemImage: "chevron.right") { model.postingIndex = index + 1 }
                        .labelStyle(.iconOnly)
                        .disabled(index == record.jobs.count - 1)
                    VStack(alignment: .leading) {
                        Text(job.title).font(.hubSection).lineLimit(1)
                        if let companyName = job.companyName {
                            Text(companyName).font(.hubCaption).foregroundStyle(.secondary)
                        }
                    }
                    Spacer()
                    Toggle("Only differences", isOn: $showsOnlyDifferences)
                    Button("Show posting", systemImage: "doc.text.magnifyingglass") {
                        details.show(.job(job.id, opensSession: false), from: .modelLab)
                    }
                }
                let fields = record.summary.fields.filter { !showsOnlyDifferences || !record.isAgreed(jobID: job.id, field: $0) }
                if fields.isEmpty {
                    ContentUnavailableView("Every stack agrees", systemImage: "equal.circle", description: Text("Turn off Only differences to see every field."))
                } else {
                    List(fields, id: \.self) { field in
                        fieldSection(field, jobID: job.id)
                    }
                }
            }
        }
    }

    private func fieldSection(_ field: String, jobID: UUID) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text(ComparisonSummary.formatFieldTitle(field)).font(.hubSection)
            ForEach(record.comparison.stacks) { stack in
                let answer = record.getAnswer(stackID: stack.id, jobID: jobID)
                let reading = answer?.readings.first { $0.field == field }
                HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                    Text(stack.label).font(.hubSecondary).foregroundStyle(.secondary).frame(width: 150, alignment: .leading)
                    VStack(alignment: .leading, spacing: 2) {
                        if let error = answer?.error {
                            Label(error, systemImage: "xmark.circle.fill").foregroundStyle(Tone.negative.color)
                        } else if answer == nil {
                            Text("Not answered yet").foregroundStyle(.secondary)
                        } else {
                            Text(reading.map { $0.text.isEmpty ? "—" : $0.text } ?? "—").textSelection(.enabled)
                            if let evidence = reading?.evidence {
                                Evidence(text: evidence)
                            }
                            if let reason = reading?.reason {
                                Text(reason).font(.hubCaption).foregroundStyle(.secondary)
                            }
                        }
                    }
                    Spacer()
                    if answer?.error == nil, answer != nil {
                        verdictButtons(stackID: stack.id, jobID: jobID, field: field)
                    }
                }
            }
        }
        .padding(.vertical, Space.xs)
    }

    private func verdictButtons(stackID: UUID, jobID: UUID, field: String) -> some View {
        let verdict = record.getVerdict(stackID: stackID, jobID: jobID, field: field)
        return HStack(spacing: Space.xs) {
            Button("Right", systemImage: verdict == .right ? "checkmark.circle.fill" : "checkmark.circle") {
                Task { await model.setVerdict(.right, stackID: stackID, jobID: jobID, field: field, with: client) }
            }
            .foregroundStyle(verdict == .right ? AnyShapeStyle(ComparisonVerdictKind.right.tone.color) : AnyShapeStyle(.secondary))
            Button("Wrong", systemImage: verdict == .wrong ? "xmark.circle.fill" : "xmark.circle") {
                Task { await model.setVerdict(.wrong, stackID: stackID, jobID: jobID, field: field, with: client) }
            }
            .foregroundStyle(verdict == .wrong ? AnyShapeStyle(ComparisonVerdictKind.wrong.tone.color) : AnyShapeStyle(.secondary))
        }
        .labelStyle(.iconOnly)
        .buttonStyle(.borderless)
        .disabled(model.isSavingVerdict)
    }
}

/// A stack being set up in the new-comparison sheet: Claude with a model, or
/// a provider with one of its models.
private struct StackDraft: Identifiable {
    let id = UUID()
    var providerID: UUID?
    var model: String
    var label: String
}

/// Picks the stacks and how many of the newest postings to run them on.
private struct NewComparisonSheet: View {
    let client: HubClient
    let start: (NewComparison) async throws -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var title = ""
    @State private var freshJobs = 3
    @State private var stacks = [StackDraft(providerID: nil, model: "sonnet", label: "Sonnet")]
    @State private var providers: [ModelProvider] = []
    @State private var modelsByProvider: [UUID: [String]] = [:]
    @State private var failure: HubFailure?

    var body: some View {
        Form {
            TextField("Title", text: $title, prompt: Text("Sonnet against the local 9B"))
            Stepper("Newest postings: \(freshJobs)", value: $freshJobs, in: 1...50)
            Section("Stacks, the first is the reference") {
                ForEach($stacks) { $stack in
                    HStack {
                        Picker("Runs on", selection: $stack.providerID) {
                            Text("Claude").tag(UUID?.none)
                            ForEach(providers) { Text($0.name).tag(UUID?.some($0.id)) }
                        }
                        .onChange(of: stack.providerID) {
                            stack.model = stack.providerID == nil ? "sonnet" : ""
                            Task { await loadModels(for: stack.providerID) }
                        }
                        modelPicker($stack)
                        TextField("Label", text: $stack.label).frame(width: 140)
                        Button("Remove", systemImage: "minus.circle") { stacks.removeAll { $0.id == stack.id } }
                            .labelStyle(.iconOnly)
                            .buttonStyle(.borderless)
                            .disabled(stacks.count <= 2)
                    }
                }
                Button("Add stack", systemImage: "plus") {
                    stacks.append(StackDraft(providerID: providers.first?.id, model: "", label: ""))
                    Task { await loadModels(for: providers.first?.id) }
                }
            }
            if let failure {
                HubErrorView(failure)
            }
        }
        .formStyle(.grouped)
        .frame(width: 640)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
            ToolbarItem(placement: .confirmationAction) {
                AsyncButton("Start", busyTitle: "Starting…") { await submit() }
                    .disabled(!canStart)
            }
        }
        .task { await loadProviders() }
    }

    private var canStart: Bool {
        stacks.count >= 2 && stacks.allSatisfy { !$0.model.isEmpty }
    }

    @ViewBuilder
    private func modelPicker(_ stack: Binding<StackDraft>) -> some View {
        if let providerID = stack.wrappedValue.providerID {
            Picker("Model", selection: stack.model) {
                Text("Choose").tag("")
                ForEach(modelsByProvider[providerID] ?? [], id: \.self) { Text($0).tag($0) }
            }
            .onChange(of: stack.wrappedValue.model) {
                if stack.wrappedValue.label.isEmpty || !stack.wrappedValue.model.isEmpty {
                    stack.wrappedValue.label = Self.makeLabel(stack.wrappedValue.model)
                }
            }
        } else {
            Picker("Model", selection: stack.model) {
                ForEach(["sonnet", "opus", "haiku"], id: \.self) { Text($0.capitalized).tag($0) }
            }
            .onChange(of: stack.wrappedValue.model) { stack.wrappedValue.label = stack.wrappedValue.model.capitalized }
        }
    }

    /// A model file's name without its extension: "Qwen3.5-9B-Q4_K_M.gguf"
    /// is labelled "Qwen3.5-9B-Q4_K_M".
    private static func makeLabel(_ model: String) -> String {
        model.hasSuffix(".gguf") ? String(model.dropLast(".gguf".count)) : model
    }

    private func loadProviders() async {
        do {
            providers = try await client.get("v1/model-providers", as: ModelProvidersResponse.self).providers
            if stacks.count == 1, let local = providers.first(where: \.isHubRuntime) ?? providers.first {
                stacks.append(StackDraft(providerID: local.id, model: "", label: ""))
                await loadModels(for: local.id)
            }
        } catch {
            failure = HubFailure("Couldn't list the providers", error)
        }
    }

    /// Lists the provider's models once, and gives its stacks without a
    /// model the first of them.
    private func loadModels(for providerID: UUID?) async {
        guard let providerID else { return }
        if modelsByProvider[providerID] == nil {
            do {
                modelsByProvider[providerID] = try await client.get("v1/model-providers/\(providerID.uuidString)/models", as: ProviderModelsResponse.self).models
            } catch {
                modelsByProvider[providerID] = []
                failure = HubFailure("Couldn't list the models", error)
            }
        }
        guard let firstModel = modelsByProvider[providerID]?.first else { return }
        for index in stacks.indices where stacks[index].providerID == providerID && stacks[index].model.isEmpty {
            stacks[index].model = firstModel
            stacks[index].label = Self.makeLabel(firstModel)
        }
    }

    private func submit() async {
        let newStacks = stacks.map { stack in
            NewComparisonStack(
                label: stack.label.isEmpty ? Self.makeLabel(stack.model) : stack.label,
                source: stack.providerID == nil ? NewComparisonStack.claudeSource : NewComparisonStack.routeSource,
                providerID: stack.providerID, model: stack.model
            )
        }
        let comparisonTitle = title.trimmingCharacters(in: .whitespaces).isEmpty ? newStacks.map(\.label).joined(separator: " against ") : title
        do {
            try await start(NewComparison(title: comparisonTitle, freshJobs: freshJobs, stacks: newStacks))
            dismiss()
        } catch {
            failure = HubFailure("Couldn't start the comparison", error)
        }
    }
}
