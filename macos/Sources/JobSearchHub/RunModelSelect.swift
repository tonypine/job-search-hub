import JobSearchHubCore
import SwiftUI

/// The model select of one run control, beside its ▶: the model menu of
/// Activity in a compact form, names and provider marks alone, set to the
/// kind's model. A model picked here runs that one run; the kind keeps its
/// own.
struct RunModelSelect: View {
    @Binding var selection: RunModelSelection
    let choices: ModelChoicesModel

    var body: some View {
        let label = selection.getLabel(providers: choices.providers, catalogs: choices.catalogs)
        let groups = selection.getGroups(providers: choices.providers, catalogs: choices.catalogs)
        Menu {
            ModelChoiceMenuItems(groups: groups, selection: selection.selected) { choice in
                selection.pick(choice.routed)
            }
            if groups.isEmpty {
                Text(choices.hasLoaded ? "No models yet. Set up a provider in Settings." : "Reading the models…")
            }
            Divider()
            ProviderSettingsButton()
        } label: {
            Label(label?.name ?? "No model", systemImage: label?.markSymbol ?? "cube")
                .lineLimit(1)
                .truncationMode(.middle)
        }
        .menuStyle(.button)
        .buttonStyle(.borderless)
        .font(.hubCaption)
        .foregroundStyle(.secondary)
        .help(describe(label))
        .accessibilityLabel("Model for this run")
        .accessibilityValue(label?.name ?? "None")
    }

    private func describe(_ label: RunKindModel?) -> String {
        let runsOn = label.map { "\($0.runsOnText). " } ?? ""
        return "\(runsOn)Choose another model for this run only; the kind keeps its own"
    }
}

/// A job section's run control: the model select, set to the kind's model
/// each time the section shows, then the section's run button, which starts
/// the run on the model chosen, nil for the kind's own. Until the job's
/// kinds are read, or when they can't be, the button stands alone.
struct JobSectionRunControl<Start: View>: View {
    let kind: String
    let jobID: UUID
    let client: HubClient
    @ViewBuilder let start: (_ chosen: RoutedModel?) -> Start
    @State private var selection: RunModelSelection?
    @State private var choices = ModelChoicesModel()

    /// The select's room at most, so the button beside it keeps its words.
    static var selectWidth: CGFloat { 120 }

    /// `selection` is what the select shows until the job's kinds are read.
    init(
        kind: String, jobID: UUID, client: HubClient, selection: RunModelSelection? = nil,
        @ViewBuilder start: @escaping (_ chosen: RoutedModel?) -> Start
    ) {
        self.kind = kind
        self.jobID = jobID
        self.client = client
        self.start = start
        _selection = State(initialValue: selection)
    }

    var body: some View {
        HStack(spacing: Space.s) {
            if let binding = Binding($selection) {
                RunModelSelect(selection: binding, choices: choices)
                    .frame(maxWidth: Self.selectWidth)
                    .fixedSize(horizontal: false, vertical: true)
            }
            start(selection?.chosen)
        }
        .task(id: jobID) { await load() }
    }

    /// Sets the select on the kind's model again, then reads the models it
    /// lists.
    private func load() async {
        let kinds = try? await client.listRunnableKinds(forJob: jobID)
        selection = kinds?.first { $0.id == kind }.map(RunModelSelection.init(kind:))
        guard selection != nil else { return }
        await choices.load(with: client, readsAverages: false)
    }
}
