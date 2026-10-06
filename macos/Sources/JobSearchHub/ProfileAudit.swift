import Foundation
import JobSearchHubCore

/// An audit of the owner's LinkedIn profile for the recruiters who search,
/// written by Claude through the bundled `hub` command.
@MainActor
@Observable
final class ProfileAudit {
    enum State: Equatable {
        case idle
        case auditing
        case audited(String)
        case failed(String)
    }

    private(set) var state: State = .idle

    func audit(with client: HubClient) async {
        guard let command = ServerAgent.hubCommand else {
            state = .failed("The app has no bundled hub command. Build it with Scripts/make-app.sh.")
            return
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            state = .failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew).")
            return
        }
        state = .auditing
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await BundledHubCommandRunner.run(command, arguments: ProfileAuditLaunch.arguments, environment: environment)
        let output = finished.output.trimmingCharacters(in: .whitespacesAndNewlines)
        if finished.status == 0, !output.isEmpty {
            state = .audited(output)
        } else {
            state = .failed(output.components(separatedBy: "\n").last { !$0.isEmpty } ?? "The audit failed (exit status \(finished.status)).")
        }
    }
}
