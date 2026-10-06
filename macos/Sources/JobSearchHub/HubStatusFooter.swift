import JobSearchHubCore
import SwiftUI

/// The sessions the footer's popover lists: running sessions from this
/// app's processes, then the most recent others from the hub.
@MainActor
@Observable
final class RecentSessionsModel {
    static let listSize = 8
    private(set) var sessions: [ClaudeSession] = []

    func load(with client: HubClient) async {
        let query = [URLQueryItem(name: "limit", value: String(Self.listSize))]
        if let answer = try? await client.get("v1/claude-sessions", query: query, as: ClaudeSessionsResponse.self) {
            sessions = answer.sessions
        }
    }
}

/// The sidebar's foot: the hub's state in three lines, whether it's
/// connected, what its local models are doing, and the session that waits
/// for you, with Pause for the models. Clicking it lists the running and
/// recent sessions; each job's and company's Session tab, and ⌘K, have them
/// too.
struct HubStatusFooter: View {
    /// The column each line's dot or symbol sits in, so the words line up.
    private static let symbolWidth: CGFloat = 12

    /// What keeps the app from the hub, as the window's banner says it.
    let problem: ConnectionProblem?
    let onOpenSession: (ClaudeSessionSubject) -> Void
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @State private var modelWork = ModelWorkModel()
    @State private var sessions = RecentSessionsModel()
    @State private var isShowingSessions = false
    private let host = ClaudeSessionHost.shared

    var body: some View {
        HStack(alignment: .top, spacing: Space.s) {
            Button {
                isShowingSessions.toggle()
            } label: {
                VStack(alignment: .leading, spacing: Space.xs) {
                    connectionLine
                    modelLine
                    sessionLine
                }
                .font(.hubCaption)
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help("Show the running and recent sessions")
            .popover(isPresented: $isShowingSessions, arrowEdge: .trailing) {
                SessionsPopover(sessions: sessions.sessions) { subject in
                    isShowingSessions = false
                    onOpenSession(subject)
                }
            }
            pauseButton
        }
        .padding(Space.s)
        .background(.quinary, in: RoundedRectangle(cornerRadius: Radius.card))
        .padding(Space.s)
        .task(id: ClientKey(hubURLText: connection.hubURLText, token: connection.token.value)) {
            guard let client = connection.makeClient() else { return }
            await sessions.load(with: client)
            await modelWork.watch(with: client, every: .seconds(5))
        }
        .onChange(of: host.runningSessionIDs) { reloadSessions() }
        .onChange(of: isShowingSessions) {
            if isShowingSessions { reloadSessions() }
        }
    }

    private var connectionLine: some View {
        let (text, tone): (String, Tone) = if problem != nil {
            ("Hub not connected", .negative)
        } else if events.isConnected {
            ("Hub connected", .positive)
        } else {
            ("Connecting to the hub…", .neutral)
        }
        return HStack(spacing: Space.s) {
            Circle().fill(tone.color).frame(width: 7, height: 7)
                .frame(width: Self.symbolWidth)
            Text(text).fontWeight(.medium)
        }
        .accessibilityElement(children: .combine)
    }

    /// What the local models are doing, or why the footer can't say or
    /// couldn't pause them; a failed pause stays until the next try.
    @ViewBuilder
    private var modelLine: some View {
        if problem == nil, let failure = modelWork.pauseFailure ?? modelWork.failure {
            HStack(spacing: Space.s) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .frame(width: Self.symbolWidth)
                Text(failure.title)
            }
            .foregroundStyle(Tone.negative.color)
            .help([failure.report.advice, failure.report.details].compactMap { $0 }.joined(separator: "\n\n"))
        } else {
            let work = problem == nil ? modelWork.work : nil
            HStack(spacing: Space.s) {
                Group {
                    if work?.running != nil && work?.paused == false {
                        ProgressView().controlSize(.mini)
                    } else {
                        Image(systemName: work?.paused == true ? "pause.circle" : "cpu")
                    }
                }
                .frame(width: Self.symbolWidth)
                Text(work?.statusLine ?? "Local models unknown")
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var sessionLine: some View {
        let runningCount = host.runningSessionIDs.count
        let waiting = host.waitingSessionName
        return HStack(spacing: Space.s) {
            SessionLamp(isRunning: runningCount > 0, activity: waiting == nil ? nil : .blocked)
                .frame(width: Self.symbolWidth)
            if let waiting {
                Text("\(waiting) waits for you").foregroundStyle(Tone.caution.color)
            } else {
                Text(runningCount == 0 ? "No session running" : runningCount == 1 ? "1 session running" : "\(runningCount) sessions running")
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private var pauseButton: some View {
        let paused = modelWork.work?.paused == true
        Button(paused ? "Resume local models" : "Pause local models", systemImage: paused ? "play.circle" : "pause.circle") {
            guard let client = connection.makeClient() else { return }
            Task { await modelWork.setPaused(!paused, with: client) }
        }
        .labelStyle(.iconOnly)
        .buttonStyle(.borderless)
        .disabled(modelWork.work == nil || modelWork.failure != nil || problem != nil)
        .help(paused ? "Resume the local models' background work" : "Pause the local models' background work; runs you start still go ahead")
    }

    private func reloadSessions() {
        guard let client = connection.makeClient() else { return }
        Task { await sessions.load(with: client) }
    }

    /// Keys the footer's reads: again with a new hub address or token.
    private struct ClientKey: Equatable {
        let hubURLText: String
        let token: String?
    }
}

/// The footer's popover: running sessions first, then the most recent
/// others. Choosing one opens its job or company on its Session tab.
private struct SessionsPopover: View {
    let sessions: [ClaudeSession]
    let onOpen: (ClaudeSessionSubject) -> Void
    private let host = ClaudeSessionHost.shared

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text("Sessions").font(.hubSection)
            ForEach(ClaudeSessionList.sort(sessions, running: host.runningSessionIDs)) { session in
                Button {
                    if let subject = session.subject { onOpen(subject) }
                } label: {
                    row(session)
                }
                .buttonStyle(.plain)
            }
            if sessions.isEmpty {
                Text("None yet").foregroundStyle(.secondary)
            }
            Text("A job's or company's Session tab starts one.")
                .font(.hubCaption).foregroundStyle(.secondary)
        }
        .padding(Space.l)
        .frame(width: 300, alignment: .leading)
    }

    private func row(_ session: ClaudeSession) -> some View {
        let isRunning = host.isRunning(session.id)
        return HStack(spacing: Space.s) {
            SessionLamp(isRunning: isRunning, activity: host.activities[session.id])
            VStack(alignment: .leading, spacing: 1) {
                Text(session.name).lineLimit(1)
                Text(isRunning ? SessionLamp.describe(host.activities[session.id]) : session.lastActiveAt.formatted(.relative(presentation: .named)))
                    .font(.hubCaption)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .help(isRunning ? "Show this session" : "Resume this session")
    }
}

/// A session's state at a glance, in its tone: accent while working, caution
/// while waiting for the owner, positive when its turn is done, an empty ring
/// when not running.
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
        (activity ?? .idle).tone.color
    }

    static func describe(_ activity: SessionActivity?) -> String {
        switch activity {
        case .working: "Working"
        case .blocked: "Waiting for you"
        case .idle, nil: "Running"
        }
    }
}
