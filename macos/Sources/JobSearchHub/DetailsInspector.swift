import JobSearchHubCore
import SwiftUI

/// What the window's details inspector shows. Pages put their selection
/// here, and the inspector sits on the split view rather than on a page: the
/// window's toolbar then gives it a section of its own, and a page's toolbar
/// stays over the page.
@MainActor
@Observable
final class DetailsInspector {
    enum Subject: Equatable {
        case job(UUID, opensSession: Bool)
        /// A pipeline card for a company rather than a posting.
        case companyApplication(companyID: UUID?)
        /// The interview that deepens the owner's knowledge base.
        case profileInterview
    }

    /// The page whose selection is shown, so a page left behind never shows
    /// its selection over another.
    private(set) var page: Page?
    private(set) var subject: Subject?

    func show(_ subject: Subject?, from page: Page) {
        self.page = page
        self.subject = subject
    }

    func hide() {
        subject = nil
    }

    func getSubject(on page: Page) -> Subject? {
        self.page == page ? subject : nil
    }
}

struct DetailsInspectorContent: View {
    let subject: DetailsInspector.Subject
    let client: HubClient
    @Environment(UnseenUpdates.self) private var unseen

    var body: some View {
        switch subject {
        case let .job(jobID, opensSession):
            JobPanel(jobID: jobID, client: client, opensSession: opensSession)
        case .profileInterview:
            ClaudeSessionPane(subject: .profile, client: client, startsOnAppear: true, openingMessage: "Let's work on my knowledge base.")
        case let .companyApplication(companyID):
            ContentUnavailableView("No job on this card", systemImage: "building.2", description: Text("This application is to a company, not a posting."))
                .task(id: companyID) {
                    if let companyID {
                        await unseen.markSeen(UpdateSelection(companyID: companyID), with: client)
                    }
                }
        }
    }
}
