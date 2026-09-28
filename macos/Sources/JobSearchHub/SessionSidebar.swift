import JobSearchHubCore
import SwiftUI

/// A request to show a session: its job or company, on the Session side,
/// resumed when it has ended. Each request is new, so asking for the same
/// session twice still opens it.
struct SessionFocus: Equatable {
    let id = UUID()
    let subject: ClaudeSessionSubject
}

@MainActor
@Observable
final class SessionSidebarModel {
    static let listSize = 8
    private(set) var sessions: [ClaudeSession] = []

    func load(with client: HubClient) async {
        let query = [URLQueryItem(name: "limit", value: String(Self.listSize))]
        if let answer = try? await client.get("v1/claude-sessions", query: query, as: ClaudeSessionsResponse.self) {
            sessions = answer.sessions
        }
    }
}

/// The sidebar's Sessions section: running sessions from this app's
/// processes, then the most recent others from the hub.
struct SessionSidebarSection: View {
    let client: HubClient
    let onOpen: (ClaudeSessionSubject) -> Void
    @State private var model = SessionSidebarModel()
    private let host = ClaudeSessionHost.shared

    var body: some View {
        Section("Sessions") {
            ForEach(ClaudeSessionList.sort(model.sessions, running: host.runningSessionIDs)) { session in
                Button {
                    if let subject = session.subject { onOpen(subject) }
                } label: {
                    row(session)
                }
                .buttonStyle(.plain)
            }
            if model.sessions.isEmpty {
                Text("None yet").foregroundStyle(.secondary)
            }
        }
        .task { await model.load(with: client) }
        .onChange(of: host.runningSessionIDs) {
            Task { await model.load(with: client) }
        }
    }

    private func row(_ session: ClaudeSession) -> some View {
        let isRunning = host.isRunning(session.id)
        return HStack(spacing: 8) {
            SessionLamp(isRunning: isRunning, activity: host.activities[session.id])
            VStack(alignment: .leading, spacing: 1) {
                Text(session.name).lineLimit(1)
                Text(isRunning ? SessionLamp.describe(host.activities[session.id]) : session.lastActiveAt.formatted(.relative(presentation: .named)))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .contentShape(Rectangle())
        .help(isRunning ? "Show this session" : "Resume this session")
    }
}

/// A session's state at a glance: blue while working, orange while waiting
/// for the owner, green when its turn is done, an empty ring when not running.
struct SessionLamp: View {
    let isRunning: Bool
    let activity: SessionActivity?

    var body: some View {
        Circle()
            .fill(isRunning ? AnyShapeStyle(color) : AnyShapeStyle(.clear))
            .strokeBorder(isRunning ? AnyShapeStyle(.clear) : AnyShapeStyle(.secondary), lineWidth: 1)
            .frame(width: 8, height: 8)
    }

    private var color: Color {
        switch activity {
        case .working: .blue
        case .blocked: .orange
        case .idle, nil: .green
        }
    }

    static func describe(_ activity: SessionActivity?) -> String {
        switch activity {
        case .working: "Working"
        case .blocked: "Waiting for you"
        case .idle, nil: "Running"
        }
    }
}
