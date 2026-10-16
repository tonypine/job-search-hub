import JobSearchHubCore
import SwiftUI

/// Activity's column of run kinds: Pause all at its head, All kinds first,
/// then the kinds by what they work on. Each row shows its state, its word
/// and its model; hovering it shows its pause, and its menu has Pause and
/// Show only failed runs.
struct RunKindsColumn: View {
    let model: RunKindsModel
    /// The kind's id; empty for All kinds.
    @Binding var selection: String?
    let client: HubClient
    let showOnlyFailed: (RunKind) -> Void

    init(model: RunKindsModel, selection: Binding<String?>, client: HubClient, showOnlyFailed: @escaping (RunKind) -> Void) {
        self.model = model
        _selection = selection
        self.client = client
        self.showOnlyFailed = showOnlyFailed
    }

    var body: some View {
        List(selection: $selection) {
            AllKindsRow(all: model.all)
                .tag("")
            ForEach(model.sections) { section in
                Section(section.group.title) {
                    ForEach(section.kinds) { kind in
                        RunKindRow(kind: kind, failedCount: model.getFailedCount(kind), kindModel: model.getModel(for: kind)) {
                            await model.setPaused(!kind.paused, kind: kind, with: client)
                        }
                        .tag(kind.id)
                        .contextMenu { menu(kind) }
                        // The hover's pause and the menu, for VoiceOver and
                        // for whoever drives the app without a pointer.
                        .accessibilityActions { actions(kind) }
                    }
                }
            }
        }
        .listStyle(.sidebar)
        // The page's own background, not a second sidebar's: the state words'
        // tones and the captions keep their contrast on it.
        .scrollContentBackground(.hidden)
        .background(.background)
        .safeAreaInset(edge: .top, spacing: 0) {
            head
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            if model.pauseFailure != nil {
                HubErrorView(Bindable(model).pauseFailure)
                    .padding(Space.s)
            } else if model.hasLoaded, let failure = model.failure {
                // A read that fails after one worked: the rows are the last
                // ones read.
                Label(failure.title, systemImage: "exclamationmark.triangle.fill")
                    .font(.hubCaption)
                    .foregroundStyle(Tone.negative.color)
                    .padding(Space.s)
            }
        }
    }

    /// The column's title and Pause all; Resume all once every kind the hub
    /// starts by itself is paused.
    private var head: some View {
        let all = model.all
        return HStack(spacing: Space.s) {
            Text("Run kinds").font(.hubSection)
            Spacer(minLength: 0)
            if model.hasLoaded {
                AsyncButton(
                    all.canPauseAll ? "Pause all" : "Resume all", busyTitle: all.canPauseAll ? "Pausing…" : "Resuming…",
                    systemImage: all.canPauseAll ? "pause.fill" : "play.fill"
                ) {
                    await model.setPaused(all.canPauseAll, kind: nil, with: client)
                }
                .controlSize(.small)
                .help(all.canPauseAll ? "Pause every kind the hub starts by itself; runs you start still go ahead" : "Resume every paused kind")
            }
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
    }

    @ViewBuilder
    private func menu(_ kind: RunKind) -> some View {
        if kind.isPausable {
            pauseButton(kind)
            Divider()
        }
        showOnlyFailedButton(kind)
    }

    @ViewBuilder
    private func actions(_ kind: RunKind) -> some View {
        if kind.isPausable {
            pauseButton(kind)
        }
        showOnlyFailedButton(kind)
    }

    private func pauseButton(_ kind: RunKind) -> some View {
        Button(kind.paused ? "Resume" : "Pause", systemImage: kind.paused ? "play" : "pause") {
            Task { await model.setPaused(!kind.paused, kind: kind, with: client) }
        }
    }

    private func showOnlyFailedButton(_ kind: RunKind) -> some View {
        Button("Show Only Failed Runs", systemImage: "exclamationmark.circle") {
            showOnlyFailed(kind)
        }
    }
}

/// All kinds' row: its state, what runs and waits, and how many of the
/// kinds the hub starts by itself are paused.
private struct AllKindsRow: View {
    let all: AllRunKinds

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            RunKindGlyph(status: all.status)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline) {
                    Text("All kinds").fontWeight(.semibold).lineLimit(1)
                    Spacer(minLength: Space.s)
                    Text(all.stateWord).font(.hubCaption).foregroundStyle(all.status == .idle ? AnyShapeStyle(.secondary) : AnyShapeStyle(all.status.tone.color))
                        .lineLimit(1)
                }
                Text(all.activityLine).font(.hubCaption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if let pausedLine = all.pausedLine {
                    Text(pausedLine).font(.hubCaption).foregroundStyle(Tone.caution.color)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
    }
}

/// A kind's row: its state's glyph, its name and word, and its model with
/// its provider's mark. Hovering a kind that pauses swaps its word for its
/// pause or resume.
private struct RunKindRow: View {
    let kind: RunKind
    let failedCount: Int
    let kindModel: RunKindModel?
    let togglePause: @MainActor () async -> Void
    @State private var isHovering = false

    var body: some View {
        let status = kind.getStatus(failedCount: failedCount)
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            RunKindGlyph(status: status)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                    // The name keeps its room; the word gives way first.
                    Text(kind.title).lineLimit(1)
                        .layoutPriority(1)
                        .help(kind.title)
                    Spacer(minLength: Space.s)
                    if isHovering && kind.isPausable {
                        AsyncButton(kind.paused ? "Resume" : "Pause", busyTitle: kind.paused ? "Resuming…" : "Pausing…",
                                    systemImage: kind.paused ? "play.fill" : "pause.fill", action: togglePause)
                            .labelStyle(.iconOnly)
                            .buttonStyle(.borderless)
                            .help(kind.paused ? "Resume \(kind.title)" : "Pause \(kind.title); runs you start still go ahead")
                    } else {
                        Text(kind.getStateWord(failedCount: failedCount))
                            .font(.hubCaption)
                            .foregroundStyle(status == .idle || status == .onDemand ? AnyShapeStyle(.secondary) : AnyShapeStyle(status.tone.color))
                            .lineLimit(1)
                            .help(status == .failed ? "Failed in the last day" : "")
                    }
                }
                if let kindModel {
                    Label(kindModel.name, systemImage: kindModel.markSymbol)
                        .font(.hubCaption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .help("Runs on \(kindModel.name) from \(kindModel.providerName)")
                }
            }
        }
        .padding(.vertical, 2)
        .contentShape(Rectangle())
        .onHover { isHovering = $0 }
    }
}

/// A kind's state at a glance: a spinner while it runs, otherwise its
/// symbol in its tone.
struct RunKindGlyph: View {
    let status: RunKindStatus

    var body: some View {
        Group {
            if status == .running {
                ProgressView().controlSize(.mini)
            } else {
                Image(systemName: status.symbolName)
                    .foregroundStyle(status.tone.color)
            }
        }
        .frame(width: 14)
        .accessibilityLabel(status.title)
    }
}
