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
        guard !isRunning(companyID) else { return }
        guard let command = Bundle.main.url(forResource: "hub", withExtension: nil) else {
            states[companyID] = .failed("The app has no bundled hub command. Build it with Scripts/make-app.sh.")
            return
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            states[companyID] = .failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew).")
            return
        }
        states[companyID] = .running
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        Task {
            let finished = await BundledHubCommandRunner.run(command, arguments: JobFinderLaunch.makeArguments(companyID: companyID), environment: environment)
            if finished.status == 0 {
                let outcome = JobFinderLaunch.parseOutcome(finished.output)
                states[companyID] = .finished(openJobs: outcome.openJobs, jobBoard: outcome.jobBoard)
            } else {
                let reason = finished.output.components(separatedBy: "\n").last { !$0.isEmpty }
                states[companyID] = .failed(reason ?? "The job finder stopped (exit status \(finished.status)).")
            }
            revision += 1
        }
    }
}
