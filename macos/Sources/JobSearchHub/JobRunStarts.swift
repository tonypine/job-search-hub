import Foundation
import JobSearchHubCore

/// The runs started from a job's or a company's Run popover, or a section's
/// run control, for as long as the app runs: each row says "Running" until
/// its run lands, on the job's page, in the inspector or on Decide alike.
@MainActor
@Observable
final class JobRunStarts {
    /// A kind's runs for one job or company.
    struct Key: Hashable {
        let subjectID: UUID
        let kind: String
    }

    struct Running {
        let runID: UUID
        let startedAt: Date
    }

    private(set) var running: [Key: Running] = [:]
    /// The starts the hub hasn't answered yet.
    private(set) var starting: Set<Key> = []
    /// Bumps as a started run lands, so the job or the company and its runs
    /// read again.
    private(set) var revision = 0
    private let pollInterval: Duration
    private let getFollowLimit: (RunnableKind) -> TimeInterval
    /// Runs the Mac's task an agent kind waits in here, at once and to its
    /// end; false when another runner has it.
    private let runTask: @MainActor (UUID, HubClient) async -> Bool

    init(
        pollInterval: Duration = .seconds(3),
        followLimit: @escaping (RunnableKind) -> TimeInterval = JobRuns.getFollowLimit(of:),
        runTask: @escaping @MainActor (UUID, HubClient) async -> Bool = { _, _ in false }
    ) {
        self.pollInterval = pollInterval
        getFollowLimit = followLimit
        self.runTask = runTask
    }

    func getRunning(_ kind: String, of subjectID: UUID) -> Running? {
        running[Key(subjectID: subjectID, kind: kind)]
    }

    /// The runs started for the job or the company that still go, by kind.
    func getRunning(of subjectID: UUID) -> [(kind: String, running: Running)] {
        running.filter { $0.key.subjectID == subjectID }.map { (kind: $0.key.kind, running: $0.value) }
    }

    /// Whether the kind is starting or running for the subject, so its ▶
    /// waits, whichever popover or section pressed it.
    func isBusy(_ kind: String, of subjectID: UUID) -> Bool {
        let key = Key(subjectID: subjectID, kind: kind)
        return starting.contains(key) || running[key] != nil
    }

    /// Starts the kind for its job or company now, whatever its pause, on
    /// the model chosen for this run, else on its kind's, then follows it
    /// until it lands. Returns the follow, which ends once the row no longer
    /// says "Running".
    @discardableResult
    func start(
        _ kind: RunnableKind, for subjectID: UUID, note: String? = nil, model: RoutedModel? = nil, with client: HubClient
    ) async throws -> Task<Void, Never> {
        let key = Key(subjectID: subjectID, kind: kind.id)
        let requestedAt = Date.now
        starting.insert(key)
        defer { starting.remove(key) }
        let request = StartRunRequest(kind: kind.id, subjectType: kind.subjectType, subjectID: subjectID, model: model, note: note)
        let started = try await client.startRun(request)
        running[key] = Running(runID: started.id, startedAt: requestedAt)
        return Task { await follow(started, of: kind, key: key, with: client) }
    }

    /// Starts the kind as the hub lists it for the subject now, as a
    /// section's run control does, which has no popover row to start from.
    @discardableResult
    func start(
        _ kindID: String, subjectType: String, for subjectID: UUID, model: RoutedModel? = nil, with client: HubClient
    ) async throws -> Task<Void, Never> {
        let key = Key(subjectID: subjectID, kind: kindID)
        starting.insert(key)
        defer { starting.remove(key) }
        let kinds = try await client.listRunnableKinds(subjectType: subjectType, id: subjectID)
        guard let kind = kinds.first(where: { $0.id == kindID }) else { throw RunNotListedError(kind: kindID) }
        return try await start(kind, for: subjectID, model: model, with: client)
    }

    /// Runs an agent's task here when this Mac gets it, and has landed once
    /// it ends. Otherwise reads the subject's kinds every few seconds until
    /// the run started has ended, or for as long as such a run can take.
    private func follow(_ started: StartedRun, of kind: RunnableKind, key: Key, with client: HubClient) async {
        defer {
            if running[key]?.runID == started.id {
                running[key] = nil
            }
            revision += 1
        }
        if let taskID = started.taskID, await runTask(taskID, client) {
            return
        }
        let deadline = Date.now.addingTimeInterval(getFollowLimit(kind))
        while Date.now < deadline {
            try? await Task.sleep(for: pollInterval)
            guard let kinds = try? await client.listRunnableKinds(subjectType: kind.subjectType, id: key.subjectID) else { continue }
            if let current = kinds.first(where: { $0.id == kind.id }), JobRuns.hasLanded(started, of: current) {
                return
            }
        }
    }
}

/// A start of a kind the hub doesn't list for the subject.
struct RunNotListedError: LocalizedError {
    let kind: String

    var errorDescription: String? { "The hub lists no \(SubjectRuns.getTitle(ofKind: kind).lowercased()) run here." }
}
