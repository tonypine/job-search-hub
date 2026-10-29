import Foundation
import JobSearchHubCore

/// The runs started from a job's Run popover, for as long as the app runs:
/// each row says "Running" until its run lands, on the job's page, in the
/// inspector or on Decide alike.
@MainActor
@Observable
final class JobRunStarts {
    struct Key: Hashable {
        let jobID: UUID
        let kind: String
    }

    struct Running {
        let runID: UUID
        let startedAt: Date
    }

    private(set) var running: [Key: Running] = [:]
    /// The starts the hub hasn't answered yet.
    private(set) var starting: Set<Key> = []
    /// Bumps as a started run lands, so the job and its runs read again.
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

    func getRunning(_ kind: String, of jobID: UUID) -> Running? {
        running[Key(jobID: jobID, kind: kind)]
    }

    /// Whether the kind is starting or running for the job, so its ▶ waits,
    /// whichever popover pressed it.
    func isBusy(_ kind: String, of jobID: UUID) -> Bool {
        let key = Key(jobID: jobID, kind: kind)
        return starting.contains(key) || running[key] != nil
    }

    /// Starts the kind for the job now, on its kind's model and whatever its
    /// pause, then follows it until it lands. Returns the follow, which ends
    /// once the row no longer says "Running".
    @discardableResult
    func start(_ kind: RunnableKind, for jobID: UUID, note: String? = nil, with client: HubClient) async throws -> Task<Void, Never> {
        let key = Key(jobID: jobID, kind: kind.id)
        let requestedAt = Date.now
        starting.insert(key)
        defer { starting.remove(key) }
        let started = try await client.startRun(StartRunRequest(kind: kind.id, subjectType: JobRuns.jobSubject, subjectID: jobID, note: note))
        running[key] = Running(runID: started.id, startedAt: requestedAt)
        return Task { await follow(started, since: requestedAt, of: kind, key: key, with: client) }
    }

    /// Runs an agent's task here when this Mac gets it, and has landed once
    /// it ends. Otherwise reads the job's runs every few seconds until the
    /// run started, or a later one of its kind, has finished, or for as long
    /// as such a run can take.
    private func follow(_ started: StartedRun, since: Date, of kind: RunnableKind, key: Key, with client: HubClient) async {
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
            guard let kinds = try? await client.listRunnableKinds(forJob: key.jobID) else { continue }
            if let current = kinds.first(where: { $0.id == kind.id }), JobRuns.hasLanded(started.id, since: since, of: current) {
                return
            }
        }
    }
}
