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
            AllKindsRow(all: model.all, isSelected: selection == "")
                .tag("")
            ForEach(model.sections) { section in
                Section(section.group.title) {
                    ForEach(section.kinds) { kind in
                        RunKindRow(
                            kind: kind, failedCount: model.getFailedCount(kind), kindModel: model.getModel(for: kind),
                            isSelected: selection == kind.id
                        ) {
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
    let isSelected: Bool

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            RunKindGlyph(status: all.status, isSelected: isSelected)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline) {
                    Text("All kinds").fontWeight(.semibold).lineLimit(1)
                    Spacer(minLength: Space.s)
                    Text(all.stateWord).font(.hubCaption).foregroundStyle(all.status.wordStyle(isSelected: isSelected))
                        .fixedSize()
                }
                Text(all.activityLine).font(.hubCaption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if let pausedLine = all.pausedLine {
                    Text(pausedLine).font(.hubCaption)
                        .foregroundStyle(isSelected ? AnyShapeStyle(.primary) : AnyShapeStyle(Tone.caution.color))
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
    }
}

/// A kind's row: its state's glyph, its name and word, and its model with
/// its provider's mark. The word goes under the name when the two don't fit
/// on one line. Hovering a kind that pauses shows its pause or resume in
/// the word's place.
private struct RunKindRow: View {
    let kind: RunKind
    let failedCount: Int
    let kindModel: RunKindModel?
    let isSelected: Bool
    let togglePause: @MainActor () async -> Void
    @State private var isHovering = false

    var body: some View {
        let status = kind.getStatus(failedCount: failedCount)
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            RunKindGlyph(status: status, isSelected: isSelected)
            VStack(alignment: .leading, spacing: 2) {
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                        name().fixedSize()
                        Spacer(minLength: Space.s)
                        word(status, pauseAlignment: .trailing)
                    }
                    // At the column's least width, beside the inspector, a
                    // long name wraps rather than being cut.
                    VStack(alignment: .leading, spacing: 2) {
                        name(lines: 2).fixedSize(horizontal: false, vertical: true)
                        word(status, pauseAlignment: .leading)
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

    private func name(lines: Int = 1) -> some View {
        Text(kind.title).lineLimit(lines)
            .help(kind.title)
    }

    /// The state's word, whole. The pause covers it while hovered, so the
    /// row keeps its layout.
    private func word(_ status: RunKindStatus, pauseAlignment: Alignment) -> some View {
        let showsPause = isHovering && kind.isPausable
        return Text(kind.getStateWord(failedCount: failedCount))
            .font(.hubCaption)
            .foregroundStyle(status.wordStyle(isSelected: isSelected))
            .fixedSize()
            .help(status == .failed ? "Failed in the last day" : "")
            .opacity(showsPause ? 0 : 1)
            .accessibilityHidden(showsPause)
            .overlay(alignment: pauseAlignment) {
                if showsPause {
                    AsyncButton(kind.paused ? "Resume" : "Pause", busyTitle: kind.paused ? "Resuming…" : "Pausing…",
                                systemImage: kind.paused ? "play.fill" : "pause.fill", action: togglePause)
                        .labelStyle(.iconOnly)
                        .buttonStyle(.borderless)
                        .fixedSize()
                        .help(kind.paused ? "Resume \(kind.title)" : "Pause \(kind.title); runs you start still go ahead")
                }
            }
    }
}

extension RunKindStatus {
    /// The style of a state's word: its tone, or the row's own text color
    /// on a selected row's highlight, accent or gray, where the tones lose
    /// their contrast.
    @MainActor
    func wordStyle(isSelected: Bool) -> AnyShapeStyle {
        if isSelected { return AnyShapeStyle(.primary) }
        return self == .idle || self == .onDemand ? AnyShapeStyle(.secondary) : AnyShapeStyle(tone.color)
    }
}

/// A kind's state at a glance: a spinner while it runs, otherwise its
/// symbol in its tone, or in the row's text color when it's selected.
struct RunKindGlyph: View {
    let status: RunKindStatus
    var isSelected = false

    var body: some View {
        Group {
            if status == .running {
                ProgressView().controlSize(.mini)
            } else {
                Image(systemName: status.symbolName)
                    .foregroundStyle(isSelected ? AnyShapeStyle(.primary) : AnyShapeStyle(status.tone.color))
            }
        }
        .frame(width: 14)
        .accessibilityLabel(status.title)
    }
}
