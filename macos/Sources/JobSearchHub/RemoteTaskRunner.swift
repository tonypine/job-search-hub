import Foundation
import JobSearchHubCore

/// Runs the work asked of the Mac, by the phone or the app itself: it claims
/// each queued task, so no second runner takes it, runs it through the
/// bundled `hub` command, and reports how it ended, which the hub turns into
/// an update both see.
@MainActor
@Observable
final class RemoteTaskRunner {
    @ObservationIgnored private let jobFinder: CompanyJobFinder
    @ObservationIgnored private var isChecking = false
    /// The jobs whose fix is running.
    private(set) var fixingJobIDs: Set<UUID> = []
    /// Moves when a fix ends, so a job's details read again.
    private(set) var fixRevision = 0
    /// The tasks running now, for the install sheet.
    private(set) var runningTasks: [UUID: RunningTask] = [:]
    /// While an install waits for running work, no queued task is claimed:
    /// it waits for the new version.
    var isPaused = false
    /// The tasks this Mac is claiming or running, and who waits for each to
    /// end: true once it ran here.
    @ObservationIgnored private var claimedTaskIDs: Set<UUID> = []
    @ObservationIgnored private var endWaiters: [UUID: [CheckedContinuation<Bool, Never>]] = [:]

    init(jobFinder: CompanyJobFinder) {
        self.jobFinder = jobFinder
    }

    func check(with client: HubClient) async {
        guard !isChecking, !isPaused else { return }
        isChecking = true
        defer { isChecking = false }
        // The work runs in a task of its own: the view's task that asked is
        // cancelled on the next update, and a cancelled task can't tell the
        // hub how its work ended.
        await Task { await claimQueued(with: client) }.value
    }

    private func claimQueued(with client: HubClient) async {
        guard let queued = try? await client.get("v1/tasks", query: [URLQueryItem(name: "status", value: "queued")], as: TasksResponse.self).tasks else {
            return
        }
        for task in queued where !isPaused {
            await claimAndRun(task.id, with: client)
        }
    }

    /// Queues a fix for the job, claimed for this Mac, and runs it here at
    /// once: another runner never sees it waiting.
    func fixJob(_ jobID: UUID, note: String, with client: HubClient) async throws {
        let task = try await client.queueJobFix(jobID, note: note, claim: true)
        Task { await runAndFinish(task, with: client) }
    }

    /// Runs a task the owner just started from this Mac here at once, as a
    /// fix asked for here always did, without waiting for a check, in a
    /// task of its own that no update cancels. A check here may have taken
    /// it first, on the update that said it was queued: then it waits for
    /// that run. Returns once the task has ended, true when it ran here;
    /// false at once when another runner has it, or while an install waits.
    @discardableResult
    func runStarted(_ taskID: UUID, with client: HubClient) async -> Bool {
        guard !isPaused else { return false }
        return await Task {
            if await claimAndRun(taskID, with: client) {
                return true
            }
            guard claimedTaskIDs.contains(taskID) else { return false }
            return await withCheckedContinuation { endWaiters[taskID, default: []].append($0) }
        }.value
    }

    /// Runs the task unless this Mac already has it or another runner
    /// claimed it first.
    @discardableResult
    private func claimAndRun(_ taskID: UUID, with client: HubClient) async -> Bool {
        guard claimedTaskIDs.insert(taskID).inserted else { return false }
        var ran = false
        defer {
            claimedTaskIDs.remove(taskID)
            endWaiters.removeValue(forKey: taskID)?.forEach { $0.resume(returning: ran) }
        }
        guard let task = try? await client.send("POST", "v1/tasks/\(taskID)/claim", body: [String: String](), as: TaskRequest.self) else {
            return false
        }
        await runAndFinish(task, with: client)
        ran = true
        return true
    }

    private func runAndFinish(_ task: TaskRequest, with client: HubClient) async {
        runningTasks[task.id] = RunningTask(id: task.id, title: Self.describe(task), startedAt: .now)
        defer { runningTasks[task.id] = nil }
        let outcome = await run(task, with: client)
        _ = try? await client.send("POST", "v1/tasks/\(task.id)/finish", body: outcome, as: TaskRequest.self)
    }

    /// What the task does, as the install sheet lists it.
    private static func describe(_ task: TaskRequest) -> String {
        switch task.kind {
        case TaskRequest.findJobs: "Finding jobs for your phone"
        case TaskRequest.researchCompany where task.companyID != nil: task.input.flatMap { $0.isEmpty ? nil : "Researching \($0)" } ?? "Researching a company"
        case TaskRequest.researchCompany: task.input.flatMap { $0.isEmpty ? nil : "Researching \($0) for your phone" } ?? "Researching a company for your phone"
        case TaskRequest.fixJob: "Fixing a job's details"
        default: "A task for your phone"
        }
    }

    private func run(_ task: TaskRequest, with client: HubClient) async -> FinishTaskRequest {
        switch task.kind {
        case TaskRequest.findJobs:
            guard let companyID = task.companyID else {
                return FinishTaskRequest(succeeded: false, result: "The request named no company.", companyID: nil)
            }
            return await findJobs(companyID, with: client, addedName: nil)
        case TaskRequest.researchCompany:
            return await research(task, with: client)
        case TaskRequest.fixJob:
            guard let jobID = task.jobID else {
                return FinishTaskRequest(succeeded: false, result: "The request named no job.", companyID: nil)
            }
            return await fix(jobID, note: task.input ?? "", companyID: task.companyID, with: client)
        default:
            return FinishTaskRequest(succeeded: false, result: "The Mac doesn't know how to \(task.kind).", companyID: nil)
        }
    }

    private func findJobs(_ companyID: UUID, with client: HubClient, addedName: String?) async -> FinishTaskRequest {
        let name = (try? await client.get("v1/companies/\(companyID)", as: CompanyDossier.self).company.name) ?? addedName ?? "The company"
        switch await jobFinder.run(companyID: companyID, client: client) {
        case let .finished(openJobs, jobBoard):
            let count = openJobs.map { $0 == 1 ? "1 open job" : "\($0) open jobs" } ?? "its open jobs"
            let headline = addedName == nil ? "\(name) has \(count)" : "Added \(name), with \(count)"
            return FinishTaskRequest(succeeded: true, result: headline + (jobBoard.map { "\nRead from \($0)." } ?? ""), companyID: companyID)
        case let .failed(reason):
            return FinishTaskRequest(succeeded: false, result: "Finding jobs at \(name) failed: \(reason)", companyID: companyID)
        case .running:
            return FinishTaskRequest(succeeded: false, result: "Finding jobs at \(name) didn't end.", companyID: companyID)
        }
    }

    private func fix(_ jobID: UUID, note: String, companyID: UUID?, with client: HubClient) async -> FinishTaskRequest {
        guard let command = ServerAgent.hubCommand, let claude = ClaudeLaunch.findClaudeExecutable() else {
            return FinishTaskRequest(succeeded: false, result: "The Mac app can't run agents: its hub command or Claude Code is missing.", companyID: companyID)
        }
        fixingJobIDs.insert(jobID)
        defer {
            fixingJobIDs.remove(jobID)
            fixRevision += 1
        }
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(command, arguments: JobFixLaunch.makeArguments(jobID: jobID, note: note), environment: environment)
        let outcome = JobFixLaunch.parseOutcome(finished.output, status: finished.status)
        let title = (try? await client.get("v1/jobs/\(jobID)", as: JobDetails.self).job.title) ?? "the job"
        if outcome.fixed {
            return FinishTaskRequest(succeeded: true, result: "\(outcome.summary)\nFrom your note on \(title): \(note)", companyID: companyID)
        }
        return FinishTaskRequest(succeeded: false, result: "Fixing \(title) failed: \(outcome.summary)", companyID: companyID)
    }

    /// Researches the company the task names, then finds its jobs. One the
    /// phone asked for is added; one the owner started research for, which
    /// the hub already has, keeps how it was found.
    private func research(_ task: TaskRequest, with client: HubClient) async -> FinishTaskRequest {
        let company = task.input ?? ""
        guard let command = ServerAgent.hubCommand, let claude = ClaudeLaunch.findClaudeExecutable() else {
            return FinishTaskRequest(succeeded: false, result: "The Mac app can't run agents: its hub command or Claude Code is missing.", companyID: nil)
        }
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(
            command, arguments: CompanyResearchLaunch.makeArguments(for: task), environment: environment
        )
        let lines = finished.output.components(separatedBy: "\n")
        guard finished.status == 0, let companyID = lines.lazy.compactMap(CompanyResearchLaunch.parseCompanyID).last else {
            let reason = lines.last { !$0.isEmpty } ?? "exit status \(finished.status)"
            return FinishTaskRequest(succeeded: false, result: "Researching \(company) failed: \(reason)", companyID: nil)
        }
        return await findJobs(companyID, with: client, addedName: task.companyID == nil ? company : nil)
    }
}
