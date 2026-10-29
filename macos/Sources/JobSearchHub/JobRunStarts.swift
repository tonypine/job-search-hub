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

    /// Starts the kind for the job now, on its kind's model and whatever its
    /// pause, then follows it until it lands. Returns the follow, which ends
    /// once the row no longer says "Running".
    @discardableResult
    func start(_ kind: RunnableKind, for jobID: UUID, note: String? = nil, with client: HubClient) async throws -> Task<Void, Never> {
        let started = try await client.startRun(StartRunRequest(kind: kind.id, subjectType: JobRuns.jobSubject, subjectID: jobID, note: note))
        let key = Key(jobID: jobID, kind: kind.id)
        running[key] = Running(runID: started.id, startedAt: .now)
        return Task { await follow(started, of: kind, key: key, before: kind.lastRun?.id, with: client) }
    }

    /// Runs an agent's task here when this Mac gets it, and has landed once
    /// it ends. Otherwise reads the job's runs every few seconds until the
    /// run started, or a later one of its kind, has finished, or for as long
    /// as such a run can take.
    private func follow(_ started: StartedRun, of kind: RunnableKind, key: Key, before: UUID?, with client: HubClient) async {
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
            if let current = kinds.first(where: { $0.id == kind.id }), JobRuns.hasLanded(started.id, of: current, before: before) {
                return
            }
        }
    }
}
