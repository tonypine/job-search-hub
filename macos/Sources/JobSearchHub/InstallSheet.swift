import AppKit
import JobSearchHubCore
import SwiftUI

/// *Install now…*: what restarts and for how long, what's running, whether
/// the database changes, and the three ways on. It reads the installer
/// live; the snapshots draw the same content from values.
struct InstallSheet: View {
    private let installer = Installer.shared

    var body: some View {
        if let target = installer.target {
            InstallSheetContent(
                version: target.version, isWaiting: installer.phase == .waiting, work: installer.work,
                changesDatabase: target.changesDatabase, reopensApp: installer.reopensApp,
                isStarting: installer.phase == .starting,
                failure: Binding(get: { installer.failure }, set: { installer.failure = $0 }),
                cancel: { Task { await installer.cancel() } },
                installAnyway: { installer.isConfirmingInstallAnyway = true },
                installWhenTheseFinish: { Task { await installer.installWhenTheseFinish() } },
                openEdit: installer.openEdit
            )
            .alert(
                "Install \(target.version.description) anyway?",
                isPresented: Binding(get: { installer.isConfirmingInstallAnyway }, set: { installer.isConfirmingInstallAnyway = $0 })
            ) {
                Button("Install anyway", role: .destructive) { Task { await installer.installAnyway() } }
                Button("Keep waiting", role: .cancel) {}
            } message: {
                Text(installer.work.cost)
            }
            // The waiting sheet keeps the drain alive and the list current;
            // a reviewing one keeps its list current.
            .task(id: installer.phase) {
                guard installer.phase == .reviewing else { return }
                while !Task.isCancelled {
                    try? await Task.sleep(for: Installer.waitInterval)
                    await installer.refreshWork()
                }
            }
        }
    }
}

/// The sheet's content, from values.
struct InstallSheetContent: View {
    let version: HubVersion
    /// *Install when these finish* was chosen: the list ticks off.
    let isWaiting: Bool
    let work: RunningWork
    let changesDatabase: Bool
    let reopensApp: Bool
    var isStarting = false
    @Binding var failure: HubFailure?
    let cancel: () -> Void
    let installAnyway: () -> Void
    let installWhenTheseFinish: () -> Void
    let openEdit: (RunningItem) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text(isWaiting ? "Installing \(version.description) when these finish" : "Install \(version.description)")
                .font(.title3.weight(.semibold))
            Text(intro)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if !work.items.isEmpty || work.idleSessionsLine != nil {
                VStack(alignment: .leading, spacing: Space.s) {
                    if !isWaiting {
                        Text("Running now").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
                    }
                    ForEach(work.items) { item in
                        InstallItemRow(item: item, openEdit: openEdit)
                    }
                    if let idle = work.idleSessionsLine {
                        HStack(spacing: Space.s) {
                            Image(systemName: "moon.zzz").frame(width: 18).foregroundStyle(.secondary).accessibilityHidden(true)
                            Text(idle)
                            Spacer(minLength: Space.s)
                            Text("reopen where they were").font(.hubCaption).foregroundStyle(.secondary)
                        }
                    }
                }
            } else {
                Text("Nothing is running.").foregroundStyle(.secondary)
            }
            if !isWaiting {
                Text(changesDatabase ? "This version changes the database. A copy is saved first." : "The database stays as it is.")
                    .font(.hubSecondary)
                    .foregroundStyle(.secondary)
            }
            if work.hasUnsavedEdits {
                Text("Save or discard the unsaved edits first: an install would lose them.")
                    .font(.hubSecondary)
                    .foregroundStyle(Tone.caution.color)
            }
            if failure != nil {
                HubErrorView($failure)
            }
            buttons
        }
        .padding(Space.l)
        .frame(width: 500)
    }

    private var intro: String {
        if isWaiting {
            return reopensApp
                ? "The hub isn't starting new work. You can keep working; a session you message again keeps the install waiting."
                : "The hub isn't starting new work. Job Search Hub quits once these finish, then installs \(version.description)."
        }
        return reopensApp
            ? "The hub's server restarts, which takes about 10 seconds, and Job Search Hub reopens. Your phone shows the hub as offline meanwhile."
            : "The hub's server restarts, which takes about 10 seconds, and Job Search Hub stays closed. Your phone shows the hub as offline meanwhile."
    }

    @ViewBuilder
    private var buttons: some View {
        HStack(spacing: Space.s) {
            Spacer()
            if isStarting {
                ProgressView().controlSize(.small)
                Text("Starting the install…").foregroundStyle(.secondary)
            } else if isWaiting {
                Button("Cancel install", action: cancel)
                    .keyboardShortcut(.cancelAction)
                Button("Install anyway…", action: installAnyway)
                    .disabled(work.hasUnsavedEdits)
            } else if work.isClear {
                Button("Cancel", action: cancel)
                    .keyboardShortcut(.cancelAction)
                Button("Install now", action: installWhenTheseFinish)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
            } else {
                Button("Cancel", action: cancel)
                    .keyboardShortcut(.cancelAction)
                Button("Install anyway…", action: installAnyway)
                    .disabled(work.hasUnsavedEdits)
                Button("Install when these finish", action: installWhenTheseFinish)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                    .disabled(work.hasUnsavedEdits)
            }
        }
        .controlSize(.large)
    }
}

/// One line of *Running now*: running, or ticked off once it ended.
private struct InstallItemRow: View {
    let item: RunningItem
    let openEdit: (RunningItem) -> Void

    var body: some View {
        HStack(spacing: Space.s) {
            icon.frame(width: 18)
            Text(item.title)
                .fontWeight(item.hasEnded ? .regular : .medium)
                .strikethrough(item.hasEnded)
                .foregroundStyle(item.hasEnded ? .secondary : .primary)
            Spacer(minLength: Space.s)
            if item.kind == .unsavedEdit && !item.hasEnded {
                Button("Open") { openEdit(item) }
                    .buttonStyle(.link)
                    .help("Show the edit, to save or discard it")
            } else {
                Text(item.hasEnded ? item.endedDetail : item.detail).font(.hubCaption).foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var icon: some View {
        if item.hasEnded {
            Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.positive.color).accessibilityHidden(true)
        } else if item.kind == .unsavedEdit {
            Image(systemName: "pencil.circle.fill").foregroundStyle(Tone.caution.color).accessibilityHidden(true)
        } else if item.kind == .session(.blocked) {
            Image(systemName: "hand.raised.circle").foregroundStyle(Tone.caution.color).accessibilityHidden(true)
        } else {
            ProgressView().controlSize(.small).accessibilityHidden(true)
        }
    }
}

/// Where the connection banner goes: how the last install ended, for a day
/// or until dismissed.
struct InstallNoticeBanner: View {
    let notice: Installer.Notice
    let showWhatsNew: () -> Void
    let showLog: () -> Void
    let dismiss: () -> Void
    @State private var isShowingCommands = false

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol).foregroundStyle(tone.color).accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).fontWeight(.medium)
                if let detail {
                    Text(detail).font(.hubSecondary).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
            }
            switch notice.outcome {
            case .installed:
                Button("What's new", action: showWhatsNew).buttonStyle(.link).fontWeight(.medium)
            case .rolledBack, .abandoned:
                Button("Show install log", action: showLog).buttonStyle(.link)
            case .rollbackFailed:
                Button("Show how to finish") { isShowingCommands = true }.buttonStyle(.link)
            }
            Spacer(minLength: Space.s)
            Button(action: dismiss) {
                Image(systemName: "xmark").font(.caption.weight(.semibold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("Dismiss")
            .accessibilityLabel("Dismiss")
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .background(tone.fill, in: RoundedRectangle(cornerRadius: Radius.card))
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .accessibilityElement(children: .contain)
        .sheet(isPresented: $isShowingCommands) {
            if case let .rollbackFailed(message, error, commands) = notice.outcome {
                FinishRollbackSheet(message: message, error: error, commands: commands, showLog: showLog)
            }
        }
    }

    private var title: String {
        switch notice.outcome {
        case let .installed(version): "Now on \(version.description)"
        case let .rolledBack(message, _): message
        case let .abandoned(message): message
        case .rollbackFailed: "The hub couldn't go back by itself"
        }
    }

    private var detail: String? {
        switch notice.outcome {
        case .installed, .abandoned: nil
        case let .rolledBack(_, lost): lost
        case let .rollbackFailed(message, _, _): message
        }
    }

    private var symbol: String {
        switch notice.outcome {
        case .installed: "checkmark.circle.fill"
        case .rolledBack: "arrow.uturn.backward.circle.fill"
        case .abandoned: "info.circle.fill"
        case .rollbackFailed: "exclamationmark.triangle.fill"
        }
    }

    private var tone: Tone {
        switch notice.outcome {
        case .installed: .positive
        case .rolledBack: .caution
        case .abandoned: .neutral
        case .rollbackFailed: .negative
        }
    }
}

/// A rollback that stopped: where, and the commands that finish it, to run
/// in Terminal.
struct FinishRollbackSheet: View {
    let message: String
    let error: String
    let commands: [String]
    let showLog: () -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("The hub couldn't go back by itself").font(.title3.weight(.semibold))
            Text(message).fixedSize(horizontal: false, vertical: true)
            Text(error).font(.hubSecondary).foregroundStyle(.secondary).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
            Text("Everything was left as it is. To finish going back, run these in Terminal, in order:").font(.hubSecondary)
            ScrollView {
                Text(commands.joined(separator: "\n"))
                    .font(.system(.callout, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(Space.s)
            }
            .frame(minHeight: 80, maxHeight: 200)
            .background(.quinary, in: RoundedRectangle(cornerRadius: Radius.card))
            HStack {
                Button("Show install log", action: showLog)
                Spacer()
                Button("Copy commands") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(commands.joined(separator: "\n"), forType: .string)
                }
                Button("Done") { dismiss() }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(Space.l)
        .frame(width: 560)
    }
}

/// While `hub-update` finishes an install this app didn't start, as one cut
/// short by a power loss.
struct InstallUnderWayBanner: View {
    let state: InstallState

    var body: some View {
        HStack(spacing: Space.s) {
            ProgressView().controlSize(.small).accessibilityHidden(true)
            Text("Finishing the install of \(state.to)… The hub restarts in a moment.").fontWeight(.medium)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .background(Tone.neutral.fill, in: RoundedRectangle(cornerRadius: Radius.card))
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
    }
}
