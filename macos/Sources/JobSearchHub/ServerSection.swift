import AppKit
import JobSearchHubCore
import SwiftUI

/// The hub server's launch agent on this Mac, read, started, stopped and
/// restarted through `launchctl`.
@MainActor
@Observable
final class ServerControl {
    private(set) var state: ServerLaunchAgent.State?
    private(set) var isWorking = false
    var failure: HubFailure?
    @ObservationIgnored private let home = FileManager.default.homeDirectoryForCurrentUser

    var logURL: URL { ServerLaunchAgent.makeLogURL(home: home) }
    private var plistURL: URL { ServerLaunchAgent.makePlistURL(home: home) }
    var bundleCarriesServer: Bool { ServerAgent.bundleCarriesServer }
    /// Whether `install-app.sh` has installed the agent, so an unloaded
    /// server can be started. It runs the installed app's server, whichever
    /// build starts it.
    var isInstalled: Bool { FileManager.default.fileExists(atPath: plistURL.path) }

    func watch() async {
        while !Task.isCancelled {
            await readState()
            try? await Task.sleep(for: .seconds(3))
        }
    }

    func readState() async {
        let finished = await runLaunchctl(.readState)
        state = ServerLaunchAgent.parseState(finished.output, status: finished.status)
    }

    /// Starts the server by loading its agent, which launchd runs at once,
    /// then reads its state again.
    func start() async {
        guard isInstalled else {
            failure = HubFailure(
                "Couldn't start the server",
                advice: "It isn't installed on this Mac. Install it with macos/Scripts/install-app.sh from the repository."
            )
            return
        }
        await perform(.start(plist: plistURL))
    }

    /// Has launchd start, stop or restart the server, then reads its state again.
    func perform(_ command: ServerLaunchAgent.Command) async {
        isWorking = true
        defer { isWorking = false }
        let finished = await runLaunchctl(command)
        failure = finished.status == 0 ? nil : HubFailure(
            "Couldn't \(describe(command)) the server", advice: "launchd turned it down. The log may say why.",
            details: "launchctl: \(finished.output.trimmingCharacters(in: .whitespacesAndNewlines))"
        )
        await readState()
    }

    private func describe(_ command: ServerLaunchAgent.Command) -> String {
        switch command {
        case .readState: "read"
        case .start: "start"
        case .stop: "stop"
        case .restart: "restart"
        }
    }

    private func runLaunchctl(_ command: ServerLaunchAgent.Command) async -> (output: String, status: Int32) {
        let arguments = ServerLaunchAgent.makeArguments(command, userID: getuid())
        return await BundledHubCommandRunner.run(URL(filePath: "/bin/launchctl"), arguments: arguments, environment: ProcessInfo.processInfo.environment)
    }
}

/// `~/.local/bin/hub`, and linking it to this bundle's `hub`. Only the
/// installed app offers it: a build in macos/build/ is deleted by the next
/// build, which would leave the link pointing at nothing.
@MainActor
@Observable
final class HubCommandControl {
    private(set) var state: HubCommandLink.State = .missing
    var failure: HubFailure?
    @ObservationIgnored private let link = HubCommandLink.makeLinkURL(home: FileManager.default.homeDirectoryForCurrentUser)
    @ObservationIgnored let target = ServerAgent.isInstalledCopy ? ServerAgent.hubCommand : nil

    func readState() {
        guard let target else { return }
        state = HubCommandLink.readState(link: link, target: target)
    }

    func install() {
        guard let target else { return }
        do {
            try HubCommandLink.install(link: link, target: target)
            failure = nil
        } catch {
            failure = HubFailure("Couldn't install the hub command", error)
        }
        readState()
    }
}

/// Settings' control panel for the server: its state, start, stop, restart
/// and log, and pausing the hub's background model work. Starting needs no
/// connection, so it works while the hub is down.
struct ServerSection: View {
    let client: HubClient?
    @State private var control = ServerControl()
    @State private var hubCommand = HubCommandControl()
    @State private var modelWork = ModelWorkModel()
    @State private var isConfirmingStop = false

    var body: some View {
        Section("Server on this Mac") {
            if !control.bundleCarriesServer {
                Text("This build of the app doesn't carry the server. Install one with macos/Scripts/install-app.sh from the repository.")
                    .foregroundStyle(.secondary)
            } else {
                Label(describeState(), systemImage: stateSymbol).foregroundStyle(control.state?.tone.color ?? Tone.neutral.color)
                HStack {
                    if control.state == .stopped {
                        if control.isInstalled {
                            AsyncButton("Start", busyTitle: "Starting…") { await control.start() }
                        } else {
                            Text("It isn't installed on this Mac. Install it with macos/Scripts/install-app.sh from the repository.")
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        AsyncButton("Restart", busyTitle: "Restarting…") { await control.perform(.restart) }
                        Button("Stop") { isConfirmingStop = true }
                            .disabled(control.isWorking)
                    }
                    Button("Show log") { NSWorkspace.shared.open(control.logURL) }
                }
                if control.failure != nil {
                    HubErrorView($control.failure)
                }
            }
            if hubCommand.target != nil {
                hubCommandRow
            }
            if client != nil, let work = modelWork.work {
                HStack {
                    Text(work.paused ? "Model work is paused" : "Model work is running")
                    Spacer()
                    AsyncButton(work.paused ? "Resume" : "Pause", busyTitle: work.paused ? "Resuming…" : "Pausing…") {
                        if let client { await modelWork.setPaused(!work.paused, with: client) }
                    }
                }
                if modelWork.pauseFailure != nil {
                    HubErrorView($modelWork.pauseFailure)
                }
                if modelWork.failure != nil {
                    HubErrorView($modelWork.failure)
                }
            }
        }
        .task { await control.watch() }
        .task { hubCommand.readState() }
        .task(id: client == nil) {
            if let client { await modelWork.watch(with: client) }
        }
        .confirmationDialog("Stop the hub?", isPresented: $isConfirmingStop) {
            Button("Stop", role: .destructive) { Task { await control.perform(.stop) } }
        } message: {
            Text("The phone, the Mac app and the agents lose the hub until you start it again, or until the Mac restarts.")
        }
    }

    /// The terminal's `hub`, linked to this app's so it is always the
    /// installed version's.
    @ViewBuilder private var hubCommandRow: some View {
        if hubCommand.state == .linked {
            Label("The hub command is installed in ~/.local/bin", systemImage: "checkmark.circle.fill")
                .foregroundStyle(Tone.positive.color)
        } else {
            VStack(alignment: .leading, spacing: 4) {
                Button("Install the hub command") { hubCommand.install() }
                Text(describeHubCommand())
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        if hubCommand.failure != nil {
            HubErrorView($hubCommand.failure)
        }
    }

    private func describeHubCommand() -> String {
        let links = "Links ~/.local/bin/hub to this app's, so hub in a terminal is always this version. ~/.local/bin needs to be on your PATH."
        switch hubCommand.state {
        case .missing, .linked: return links
        case let .other(destination?): return links + " It replaces the link to \(destination)."
        case .other(destination: nil): return links + " It replaces the hub there now."
        }
    }

    private func describeState() -> String {
        switch control.state {
        case let .running(pid): "Running (process \(pid))"
        case let .waiting(lastExitCode): "Not running; launchd restarts it" + (lastExitCode.map { " (last exit code \($0))" } ?? "")
        case .stopped: "Stopped"
        case nil: "Checking…"
        }
    }

    private var stateSymbol: String {
        switch control.state {
        case .running: "checkmark.circle.fill"
        case .waiting: "arrow.clockwise.circle"
        case .stopped: "stop.circle"
        case nil: "circle.dotted"
        }
    }
}
