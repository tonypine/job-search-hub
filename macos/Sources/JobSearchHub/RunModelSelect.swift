import JobSearchHubCore
import SwiftUI

/// The model select of one run control, beside its ▶: the kind's model, or
/// the one picked, which opens the model menu of Activity in a compact
/// form, names and provider marks alone. A model picked here runs that one
/// run; the kind keeps its own. Its label needs the providers alone; the
/// providers' models are read only once the menu opens, as the hub asks
/// openrouter.ai for OpenRouter's each time.
struct RunModelSelect: View {
    @Binding var selection: RunModelSelection
    let choices: ModelChoicesModel
    let client: HubClient
    @State private var isPicking = false

    var body: some View {
        let label = selection.getLabel(providers: choices.providers, catalogs: choices.catalogs)
        Button {
            isPicking = true
        } label: {
            HStack(spacing: Space.xs) {
                Label(label?.name ?? "No model", systemImage: label?.markSymbol ?? "cube")
                    .lineLimit(1)
                    .truncationMode(.middle)
                Image(systemName: "chevron.down").font(.caption2.weight(.semibold))
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
            .contentShape(Rectangle())
        }
        .buttonStyle(.borderless)
        .help(describe(label))
        .accessibilityLabel("Model for this run")
        .accessibilityValue(label?.name ?? "None")
        .popover(isPresented: $isPicking, arrowEdge: .bottom) {
            ModelPicker(
                groups: selection.getGroups(providers: choices.providers, catalogs: choices.catalogs), selection: selection.selected,
                isLoading: !choices.hasLoaded, unlistedProviders: choices.unlistedProviders
            ) { choice in
                selection.pick(choice.routed)
                isPicking = false
            } footer: {
                if selection.isAgent {
                    Text("An agent runs on your Claude subscription, so only Claude's models are here.")
                        .font(.hubCaption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                ProviderSettingsButton()
                    .buttonStyle(.link)
            }
            .task { await choices.load(with: client, readsAverages: false) }
        }
    }

    private func describe(_ label: RunKindModel?) -> String {
        let runsOn = label.map { "\($0.runsOnText). " } ?? ""
        return "\(runsOn)Choose another model for this run only; the kind keeps its own"
    }

    /// The select on the kind's model, from the job's kinds, and the
    /// providers, which name and mark it. It reads none of the providers'
    /// models, so a section that shows on every job downloads no catalog.
    /// Nil when the kinds can't be read or the job has no such kind.
    static func prepare(kind: String, forJob jobID: UUID, choices: ModelChoicesModel, with client: HubClient) async -> RunModelSelection? {
        await prepare(kind: kind, for: .job(jobID), choices: choices, with: client)
    }

    /// The select on the kind's model, from the job's or the company's
    /// kinds, and the providers. Nil when the kinds can't be read or the
    /// subject has no such kind.
    static func prepare(
        kind: String, for subject: SubjectRunsModel.Subject, choices: ModelChoicesModel, with client: HubClient
    ) async -> RunModelSelection? {
        async let kinds = try? client.listRunnableKinds(subjectType: subject.type, id: subject.id)
        await choices.loadProviders(with: client)
        return await kinds?.first { $0.id == kind }.map(RunModelSelection.init(kind:))
    }
}

/// A job's or a company's section's run control: the model select, set to
/// the kind's model each time the section shows, then the section's run
/// button, which starts the run on the model chosen, nil for the kind's
/// own. Until the subject's kinds are read, or when they can't be, the
/// button stands alone.
struct SectionRunControl<Start: View>: View {
    let kind: String
    let subject: SubjectRunsModel.Subject
    let client: HubClient
    @ViewBuilder let start: (_ chosen: RoutedModel?) -> Start
    @State private var selection: RunModelSelection?
    @State private var choices = ModelChoicesModel()

    /// The select's room at most, so the button beside it keeps its words.
    static var selectWidth: CGFloat { 120 }

    /// `selection` is what the select shows until the subject's kinds are
    /// read.
    init(
        kind: String, subject: SubjectRunsModel.Subject, client: HubClient, selection: RunModelSelection? = nil,
        @ViewBuilder start: @escaping (_ chosen: RoutedModel?) -> Start
    ) {
        self.kind = kind
        self.subject = subject
        self.client = client
        self.start = start
        _selection = State(initialValue: selection)
    }

    var body: some View {
        HStack(spacing: Space.s) {
            if let binding = Binding($selection) {
                RunModelSelect(selection: binding, choices: choices, client: client)
                    .frame(maxWidth: Self.selectWidth)
                    .fixedSize(horizontal: false, vertical: true)
            }
            start(selection?.chosen)
        }
        .task(id: subject) {
            selection = await RunModelSelect.prepare(kind: kind, for: subject, choices: choices, with: client)
        }
    }
}
