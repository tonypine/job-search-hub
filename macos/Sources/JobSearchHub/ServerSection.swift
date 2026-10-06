import AppKit
import JobSearchHubCore
import ServiceManagement
import SwiftUI

/// The hub server's launch agent on this Mac, read, stopped and restarted
/// through `launchctl`, and started by registering it from the bundle.
@MainActor
@Observable
final class ServerControl {
    private(set) var state: ServerLaunchAgent.State?
    private(set) var isWorking = false
    var failure: HubFailure?
    @ObservationIgnored private let home = FileManager.default.homeDirectoryForCurrentUser

    var logURL: URL { ServerLaunchAgent.makeLogURL(home: home) }
    var bundleCarriesServer: Bool { ServerAgent.bundleCarriesServer }
    /// Whether this build can start an unloaded server: only the installed
    /// app registers the agent. Any build stops and restarts a loaded one.
    var canRegister: Bool { ServerAgent.isInstalledCopy }

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

    /// Starts the server by registering its agent from this bundle, which
    /// launchd loads and runs, then reads its state again.
    func start() async {
        isWorking = true
        defer { isWorking = false }
        do {
            if try await ServerAgent.register() == .requiresApproval {
                failure = HubFailure(
                    "Couldn't start the server",
                    advice: "It's turned off in System Settings › General › Login Items. Turn on Job Search Hub there, then start it again."
                )
                SMAppService.openSystemSettingsLoginItems()
            } else {
                failure = nil
            }
        } catch {
            failure = HubFailure("Couldn't start the server", error)
        }
        await readState()
    }

    /// Stops or restarts the server, then reads its state again.
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

/// Settings' control panel for the server, as status rows: the server's
/// state with Start, or Restart with Stop and its log in a menu, and the
/// terminal's hub command. Starting needs no connection, so it works while
/// the hub is down.
struct ServerSection: View {
    @State private var control = ServerControl()
    @State private var hubCommand = HubCommandControl()
    @State private var isConfirmingStop = false

    var body: some View {
        Section {
            StatusRow(
                "Server on this Mac", symbol: "server.rack", tileTone: .neutral, state: stateTitle, stateTone: control.state?.tone ?? .neutral,
                detail: stateDetail,
                help: control.bundleCarriesServer
                    ? "The hub's server runs in the background on this Mac, and launchd starts it again when it stops or the Mac restarts. "
                        + "The phone, this app and the agents reach the hub through it."
                    : "This build of the app doesn't carry the server. Install one with macos/Scripts/install-app.sh from the repository."
            ) {
                serverAction
            }
            if control.failure != nil {
                HubErrorView($control.failure)
            }
            if hubCommand.target != nil {
                StatusRow(
                    "hub command", symbol: "terminal.fill", tileTone: .neutral, state: hubCommandState.title, stateTone: hubCommandState.tone,
                    detail: "~/.local/bin/hub", help: describeHubCommand()
                ) {
                    if hubCommand.state != .linked {
                        Button("Install") { hubCommand.install() }
                    }
                }
                if hubCommand.failure != nil {
                    HubErrorView($hubCommand.failure)
                }
            }
        }
        .task { await control.watch() }
        .task { hubCommand.readState() }
        .confirmationDialog("Stop the hub?", isPresented: $isConfirmingStop) {
            Button("Stop", role: .destructive) { Task { await control.perform(.stop) } }
        } message: {
            Text("The phone, the Mac app and the agents lose the hub until you start it again, or until the Mac restarts.")
        }
    }

    /// Start while stopped; Restart while loaded, with Stop and the log in
    /// its menu.
    @ViewBuilder
    private var serverAction: some View {
        if control.bundleCarriesServer {
            if control.isWorking {
                ProgressView().controlSize(.small)
            } else if control.state == .stopped {
                if control.canRegister {
                    Button("Start") { Task { await control.start() } }
                } else {
                    Button("Show log") { NSWorkspace.shared.open(control.logURL) }
                        .help("Start it from the installed app in ~/Applications.")
                }
            } else if control.state != nil {
                Menu("Restart") {
                    Button("Stop…") { isConfirmingStop = true }
                    Button("Show log") { NSWorkspace.shared.open(control.logURL) }
                } primaryAction: {
                    Task { await control.perform(.restart) }
                }
                .menuStyle(.button)
                .fixedSize()
            }
        }
    }

    private var stateTitle: String {
        guard control.bundleCarriesServer else { return "Not in this build" }
        switch control.state {
        case .running: return "Running"
        case .waiting: return "Restarting"
        case .stopped: return "Stopped"
        case nil: return "Checking…"
        }
    }

    private var stateDetail: String? {
        guard control.bundleCarriesServer else { return nil }
        switch control.state {
        case let .running(pid): return "process \(pid)"
        case let .waiting(lastExitCode): return lastExitCode.map { "launchd restarts it · last exit code \($0)" } ?? "launchd restarts it"
        case .stopped: return control.canRegister ? nil : "Start it from the installed app in ~/Applications"
        case nil: return nil
        }
    }

    private var hubCommandState: (title: String, tone: Tone) {
        switch hubCommand.state {
        case .linked: ("Installed", .positive)
        case .missing: ("Not installed", .neutral)
        case .other: ("Another version", .caution)
        }
    }

    private func describeHubCommand() -> String {
        let links = "Links ~/.local/bin/hub to this app's, so hub in a terminal is always this version. ~/.local/bin needs to be on your PATH."
        switch hubCommand.state {
        case .missing, .linked: return links
        case let .other(destination?): return links + " Installing replaces the link to \(destination)."
        case .other(destination: nil): return links + " Installing replaces the hub there now."
        }
    }
}
