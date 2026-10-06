import Foundation
import JobSearchHubCore

/// Finds companies' jobs with the job finder agent, through the bundled `hub`
/// command, in the background. It lives as long as the app, so a run goes on
/// while the owner moves between pages.
@MainActor
@Observable
final class CompanyJobFinder {
    enum State: Equatable {
        case running
        case finished(openJobs: Int?, jobBoard: String?)
        case failed(String)
    }

    private(set) var states: [UUID: State] = [:]
    /// Bumps when a run finishes, so the pages showing jobs read again.
    private(set) var revision = 0

    func isRunning(_ companyID: UUID) -> Bool {
        states[companyID] == .running
    }

    func start(companyID: UUID, client: HubClient) {
        Task { _ = await run(companyID: companyID, client: client) }
    }

    /// Runs the job finder on the company to its end and returns how it went;
    /// a run already going on the company is waited for, not repeated.
    func run(companyID: UUID, client: HubClient) async -> State {
        if isRunning(companyID) {
            while isRunning(companyID) {
                try? await Task.sleep(for: .seconds(2))
            }
            return states[companyID] ?? .failed("The run ended without a result.")
        }
        guard let command = ServerAgent.hubCommand else {
            return finish(companyID, .failed("The app has no bundled hub command. Build it with Scripts/make-app.sh."))
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            return finish(companyID, .failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew)."))
        }
        states[companyID] = .running
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(command, arguments: JobFinderLaunch.makeArguments(companyID: companyID), environment: environment)
        if finished.status == 0 {
            let outcome = JobFinderLaunch.parseOutcome(finished.output)
            return finish(companyID, .finished(openJobs: outcome.openJobs, jobBoard: outcome.jobBoard))
        }
        let reason = finished.output.components(separatedBy: "\n").last { !$0.isEmpty }
        return finish(companyID, .failed(reason ?? "The job finder stopped (exit status \(finished.status))."))
    }

    private func finish(_ companyID: UUID, _ state: State) -> State {
        states[companyID] = state
        revision += 1
        return state
    }
}
