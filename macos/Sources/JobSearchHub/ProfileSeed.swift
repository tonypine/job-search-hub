import Foundation
import JobSearchHubCore
import SwiftUI

/// Builds the knowledge base from the CV and the LinkedIn export with the
/// seeding agent, through the bundled `hub` command. It lives as long as the
/// app, so a run goes on while the owner moves between pages.
@MainActor
@Observable
final class ProfileSeed {
    enum State: Equatable {
        case idle
        case building
        case built(added: Int, updated: Int)
        case failed(String)
    }

    private(set) var state: State = .idle
    /// Bumps when a run ends, so the knowledge base reads again.
    private(set) var revision = 0

    func start(with client: HubClient) {
        guard state != .building else { return }
        Task { await build(with: client) }
    }

    private func build(with client: HubClient) async {
        guard let command = Bundle.main.url(forResource: "hub", withExtension: nil) else {
            return finish(.failed("The app has no bundled hub command. Build it with Scripts/make-app.sh."))
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            return finish(.failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew)."))
        }
        state = .building
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(command, arguments: ProfileSeedLaunch.arguments, environment: environment)
        if finished.status == 0, let outcome = ProfileSeedLaunch.parseOutcome(finished.output) {
            return finish(.built(added: outcome.added, updated: outcome.updated))
        }
        let reason = finished.output.components(separatedBy: "\n").last { !$0.isEmpty }
        finish(.failed(reason ?? "The build stopped (exit status \(finished.status))."))
    }

    private func finish(_ state: State) {
        self.state = state
        revision += 1
    }
}
