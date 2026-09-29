import Foundation
import JobSearchHubCore

/// Runs the work the phone asks of the Mac: it claims each queued task, so no
/// second runner takes it, runs it through the bundled `hub` command, and
/// reports how it ended, which the hub turns into an update both see.
@MainActor
@Observable
final class RemoteTaskRunner {
    @ObservationIgnored private let jobFinder: CompanyJobFinder
    @ObservationIgnored private var isChecking = false

    init(jobFinder: CompanyJobFinder) {
        self.jobFinder = jobFinder
    }

    func check(with client: HubClient) async {
        guard !isChecking else { return }
        isChecking = true
        defer { isChecking = false }
        guard let queued = try? await client.get("v1/tasks", query: [URLQueryItem(name: "status", value: "queued")], as: TasksResponse.self).tasks else {
            return
        }
        for task in queued {
            guard (try? await client.send("POST", "v1/tasks/\(task.id)/claim", body: [String: String](), as: TaskRequest.self)) != nil else {
                continue
            }
            let outcome = await run(task, with: client)
            _ = try? await client.send("POST", "v1/tasks/\(task.id)/finish", body: outcome, as: TaskRequest.self)
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
            return await research(task.input ?? "", with: client)
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

    private func research(_ company: String, with client: HubClient) async -> FinishTaskRequest {
        guard let command = Bundle.main.url(forResource: "hub", withExtension: nil), let claude = ClaudeLaunch.findClaudeExecutable() else {
            return FinishTaskRequest(succeeded: false, result: "The Mac app can't run agents: its hub command or Claude Code is missing.", companyID: nil)
        }
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(
            command, arguments: CompanyResearchLaunch.makeArguments(company: company, foundVia: "Asked for from the phone"), environment: environment
        )
        let lines = finished.output.components(separatedBy: "\n")
        guard finished.status == 0, let companyID = lines.lazy.compactMap(CompanyResearchLaunch.parseCompanyID).last else {
            let reason = lines.last { !$0.isEmpty } ?? "exit status \(finished.status)"
            return FinishTaskRequest(succeeded: false, result: "Researching \(company) failed: \(reason)", companyID: nil)
        }
        return await findJobs(companyID, with: client, addedName: company)
    }
}
