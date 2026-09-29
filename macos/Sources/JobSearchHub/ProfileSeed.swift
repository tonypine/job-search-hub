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

/// The knowledge base on the Profile page: how many entries it holds and how
/// many are confirmed, and the button that builds it from the CV and LinkedIn.
struct KnowledgeBaseSection: View {
    let client: HubClient
    @Environment(ProfileSeed.self) private var seed
    @State private var entries: [ProfileEntry] = []
    @State private var errorMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Knowledge base").font(.title3.weight(.semibold))
                    Text("Your roles, cases of work and skills, which briefs and CVs draw on. Only confirmed entries speak for you.")
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button(entries.isEmpty ? "Build from CV and LinkedIn" : "Rebuild from CV and LinkedIn", systemImage: "square.stack.3d.up") {
                    seed.start(with: client)
                }
                .disabled(seed.state == .building)
            }
            switch seed.state {
            case .building:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Reading your CV and LinkedIn… this takes a few minutes; you can leave this page.")
                }
            case let .built(added, updated):
                Label(
                    added == 0 && updated == 0 ? "Nothing new in your CV and LinkedIn." : "Added \(added) entries, updated \(updated). Anything to settle is in Updates.",
                    systemImage: "checkmark.circle.fill"
                )
                .foregroundStyle(.green)
            case let .failed(reason):
                Label(reason, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            case .idle:
                EmptyView()
            }
            if let errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
            Text(describeCounts()).foregroundStyle(.secondary)
        }
        .task(id: seed.revision) { await load() }
    }

    private func describeCounts() -> String {
        if entries.isEmpty { return "No entries yet." }
        let confirmedCount = entries.count { $0.isConfirmed }
        let roleCount = entries.count { $0.kind == "role" }
        let caseCount = entries.count { $0.kind == "case" }
        return "\(entries.count) entries: \(roleCount) roles, \(caseCount) cases. \(confirmedCount) confirmed."
    }

    private func load() async {
        do {
            entries = try await client.get("v1/profile/entries", as: ProfileEntriesResponse.self).entries
            errorMessage = nil
        } catch {
            errorMessage = "Could not load the knowledge base: \(error)"
        }
    }
}
