import JobSearchHubCore
import SwiftTerm
import SwiftUI

@MainActor
@Observable
final class ClaudeSessionPaneModel {
    private(set) var sessions: [ClaudeSession] = []
    private(set) var isStarting = false
    var errorMessage: String?

    func load(_ subject: ClaudeSessionSubject, with client: HubClient) async {
        do {
            sessions = try await client.get("v1/claude-sessions", query: subject.queryItems, as: ClaudeSessionsResponse.self).sessions
        } catch {
            errorMessage = String(describing: error)
        }
    }

    /// Creates a session about the subject, then starts it.
    func startNew(_ subject: ClaudeSessionSubject, with client: HubClient, host: ClaudeSessionHost) async {
        await run {
            let session = try await client.send("POST", "v1/claude-sessions", body: CreateClaudeSessionRequest(subject), as: ClaudeSession.self)
            sessions.insert(session, at: 0)
            try await host.start(session, with: client)
        }
    }

    func resume(_ session: ClaudeSession, with client: HubClient, host: ClaudeSessionHost) async {
        await run { try await host.start(session, with: client) }
    }

    private func run(_ work: () async throws -> Void) async {
        isStarting = true
        defer { isStarting = false }
        do {
            try await work()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

/// The Session side of a job or company: its running Claude session in a
/// terminal, or a way to resume the last one or start a new one.
struct ClaudeSessionPane: View {
    let subject: ClaudeSessionSubject
    let client: HubClient
    /// Resumes the last session, or starts one, as soon as the pane shows.
    var startsOnAppear = false
    @State private var model = ClaudeSessionPaneModel()
    private let host = ClaudeSessionHost.shared

    var body: some View {
        Group {
            if let running = model.sessions.first(where: { host.isRunning($0.id) }), let terminal = host.getTerminal(for: running.id) {
                VStack(spacing: 0) {
                    HStack {
                        SessionLamp(isRunning: true, activity: host.activities[running.id])
                        Text(running.name).lineLimit(1)
                        Text(SessionLamp.describe(host.activities[running.id])).foregroundStyle(.secondary)
                        Spacer()
                        Button("Stop", systemImage: "stop.fill") { host.stop(running.id) }
                    }
                    .padding(8)
                    Divider()
                    TerminalHostView(terminal: terminal)
                }
                .onAppear { host.shownSessionIDs.insert(running.id) }
                .onDisappear { host.shownSessionIDs.remove(running.id) }
            } else {
                startOptions
            }
        }
        .task(id: subject) {
            await model.load(subject, with: client)
            if startsOnAppear, !model.sessions.contains(where: { host.isRunning($0.id) }) {
                if let latest = model.sessions.first {
                    await model.resume(latest, with: client, host: host)
                } else {
                    await model.startNew(subject, with: client, host: host)
                }
            }
        }
        .onChange(of: host.runningSessionIDs) {
            Task { await model.load(subject, with: client) }
        }
    }

    private var startOptions: some View {
        VStack(spacing: 14) {
            Image(systemName: "terminal").font(.largeTitle).foregroundStyle(.secondary)
            if let latest = model.sessions.first {
                Text(latest.name).font(.headline)
                Text("Last active \(latest.lastActiveAt.formatted(.relative(presentation: .named)))").foregroundStyle(.secondary)
                Button("Resume session", systemImage: "play.fill") { Task { await model.resume(latest, with: client, host: host) } }
                    .keyboardShortcut(.defaultAction)
                Button("Start a new session") { Task { await model.startNew(subject, with: client, host: host) } }
            } else {
                Text("No session yet").font(.headline)
                Text("A Claude session here starts knowing your profile and everything the hub has on this.")
                    .foregroundStyle(.secondary).multilineTextAlignment(.center)
                Button("Start session", systemImage: "play.fill") { Task { await model.startNew(subject, with: client, host: host) } }
                    .keyboardShortcut(.defaultAction)
            }
            if model.isStarting {
                ProgressView().controlSize(.small)
            }
            if let errorMessage = model.errorMessage {
                Text(errorMessage).foregroundStyle(.red).multilineTextAlignment(.center)
            }
        }
        .disabled(model.isStarting)
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// Shows a terminal the host owns. The terminal moves into whichever
/// container is on screen and survives the container going away.
struct TerminalHostView: NSViewRepresentable {
    let terminal: LocalProcessTerminalView

    func makeNSView(context: Context) -> NSView {
        let container = NSView()
        attach(to: container)
        return container
    }

    func updateNSView(_ container: NSView, context: Context) {
        if terminal.superview !== container {
            attach(to: container)
        }
    }

    private func attach(to container: NSView) {
        terminal.removeFromSuperview()
        terminal.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(terminal)
        NSLayoutConstraint.activate([
            terminal.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            terminal.trailingAnchor.constraint(equalTo: container.trailingAnchor),
            terminal.topAnchor.constraint(equalTo: container.topAnchor),
            terminal.bottomAnchor.constraint(equalTo: container.bottomAnchor),
        ])
        DispatchQueue.main.async { [weak terminal] in
            guard let terminal else { return }
            terminal.window?.makeFirstResponder(terminal)
        }
    }
}
