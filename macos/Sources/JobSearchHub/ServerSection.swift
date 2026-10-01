import AppKit
import JobSearchHubCore
import SwiftUI

/// The hub server's launch agent on this Mac, read and driven through
/// `launchctl`.
@MainActor
@Observable
final class ServerControl {
    private(set) var state: ServerLaunchAgent.State?
    private(set) var isWorking = false
    private(set) var errorMessage: String?
    @ObservationIgnored private let home = FileManager.default.homeDirectoryForCurrentUser

    var plistURL: URL { ServerLaunchAgent.makePlistURL(home: home) }
    var logURL: URL { ServerLaunchAgent.makeLogURL(home: home) }
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

    /// Starts, stops or restarts the server, then reads its state again.
    func perform(_ action: ServerLaunchAgent.Action) async {
        isWorking = true
        defer { isWorking = false }
        let finished = await runLaunchctl(action)
        errorMessage = finished.status == 0 ? nil : "launchctl: \(finished.output.trimmingCharacters(in: .whitespacesAndNewlines))"
        await readState()
    }

    private func runLaunchctl(_ action: ServerLaunchAgent.Action) async -> (output: String, status: Int32) {
        let arguments = ServerLaunchAgent.makeArguments(action, userID: getuid(), plist: plistURL)
        return await BundledHubCommandRunner.run(URL(filePath: "/bin/launchctl"), arguments: arguments, environment: ProcessInfo.processInfo.environment)
    }
}

/// Settings' control panel for the server: its state, start, stop, restart
/// and log, and pausing the hub's background model work.
struct ServerSection: View {
    let client: HubClient
    @State private var control = ServerControl()
    @State private var modelWork = ModelWorkModel()
    @State private var isConfirmingStop = false

    var body: some View {
        Section("Server on this Mac") {
            if !control.isInstalled {
                Text("The server isn't installed here. Run server/scripts/install-native-server.sh from the repository.")
                    .foregroundStyle(.secondary)
            } else {
                Label(describeState(), systemImage: stateSymbol).foregroundStyle(stateColor)
                HStack {
                    if control.state == .stopped {
                        Button("Start") { Task { await control.perform(.start) } }
                    } else {
                        Button("Restart") { Task { await control.perform(.restart) } }
                        Button("Stop") { isConfirmingStop = true }
                    }
                    Button("Show log") { NSWorkspace.shared.open(control.logURL) }
                    if control.isWorking {
                        ProgressView().controlSize(.small)
                    }
                }
                .disabled(control.isWorking)
                if let errorMessage = control.errorMessage {
                    Text(errorMessage).foregroundStyle(.red)
                }
            }
            if let work = modelWork.work {
                HStack {
                    Text(work.paused ? "Model work is paused" : "Model work is running")
                    Spacer()
                    Button(work.paused ? "Resume" : "Pause") { Task { await modelWork.setPaused(!work.paused, with: client) } }
                        .disabled(modelWork.isChangingPause)
                }
            }
        }
        .task { await control.watch() }
        .task { await modelWork.watch(with: client) }
        .confirmationDialog("Stop the hub?", isPresented: $isConfirmingStop) {
            Button("Stop", role: .destructive) { Task { await control.perform(.stop) } }
        } message: {
            Text("The phone, the Mac app and the agents lose the hub until you start it again, or until the Mac restarts.")
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

    private var stateColor: Color {
        switch control.state {
        case .running: .green
        case .waiting: .orange
        case .stopped, nil: .secondary
        }
    }
}
