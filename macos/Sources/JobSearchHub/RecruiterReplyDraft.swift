import AppKit
import Foundation
import JobSearchHubCore

/// A reply to a recruiter, drafted by Claude through the bundled `hub`
/// command, for the owner to copy and send on LinkedIn.
@MainActor
@Observable
final class RecruiterReplyDraft {
    enum State: Equatable {
        case idle
        case drafting
        case drafted(String)
        case failed(String)
    }

    private(set) var state: State = .idle
    /// The conversation the state is about.
    private(set) var conversationID: UUID?

    func draft(conversationID: UUID, client: HubClient) async {
        self.conversationID = conversationID
        guard let command = Bundle.main.url(forResource: "hub", withExtension: nil) else {
            state = .failed("The app has no bundled hub command. Build it with Scripts/make-app.sh.")
            return
        }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else {
            state = .failed("Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew).")
            return
        }
        state = .drafting
        let environment = BundledHubCommand.makeEnvironment(
            from: ProcessInfo.processInfo.environment, hubURL: client.baseURL, ownerToken: client.token, claude: claude
        )
        let finished = await Self.run(command, arguments: RecruiterReplyLaunch.makeArguments(conversationID: conversationID), environment: environment)
        guard self.conversationID == conversationID else { return }
        let output = finished.output.trimmingCharacters(in: .whitespacesAndNewlines)
        if finished.status == 0, !output.isEmpty {
            state = .drafted(output)
        } else {
            state = .failed(output.components(separatedBy: "\n").last { !$0.isEmpty } ?? "The draft failed (exit status \(finished.status)).")
        }
    }

    func copyDraft() {
        guard case let .drafted(text) = state else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }

    /// Runs the command to its end off the main thread, returning what it
    /// printed and how it exited.
    private nonisolated static func run(_ command: URL, arguments: [String], environment: [String: String]) async -> (output: String, status: Int32) {
        await withCheckedContinuation { continuation in
            let process = Process()
            process.executableURL = command
            process.arguments = arguments
            process.environment = environment
            let output = Pipe()
            process.standardOutput = output
            process.standardError = output
            process.terminationHandler = { finished in
                let text = String(decoding: output.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
                continuation.resume(returning: (text, finished.terminationStatus))
            }
            do {
                try process.run()
            } catch {
                continuation.resume(returning: ("Could not start the draft: \(error.localizedDescription)", -1))
            }
        }
    }
}
