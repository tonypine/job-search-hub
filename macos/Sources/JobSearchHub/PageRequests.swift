import JobSearchHubCore
import SwiftUI

/// What the ⌘K palette asks of a page it switches to: open one of its Add
/// sheets, or take the keyboard so its list can be worked from there.
enum PageRequest: Equatable {
    case addCompany
    case addCompanyFromSuggestions
    case addJobByURL
    case focusList
}

/// The palette's request waiting for its page. The page takes it when it
/// shows, or at once when it already shows.
@MainActor
@Observable
final class PageRequests {
    struct Pending: Equatable {
        let id = UUID()
        let page: Page
        let request: PageRequest
    }

    private(set) var pending: Pending?

    func ask(_ request: PageRequest, on page: Page) {
        pending = Pending(page: page, request: request)
    }

    /// The request waiting for the page, which no longer waits once taken.
    func take(for page: Page) -> PageRequest? {
        guard let pending, pending.page == page else { return nil }
        self.pending = nil
        return pending.request
    }
}

extension View {
    /// Runs the palette's requests for the page, as it shows and as they come.
    func onPageRequest(_ page: Page, perform: @escaping (PageRequest) -> Void) -> some View {
        modifier(PageRequestHandler(page: page, perform: perform))
    }
}

private struct PageRequestHandler: ViewModifier {
    let page: Page
    let perform: (PageRequest) -> Void
    @Environment(PageRequests.self) private var requests

    func body(content: Content) -> some View {
        content.onChange(of: requests.pending, initial: true) {
            guard let request = requests.take(for: page) else { return }
            // A turn later, once the palette that asked has gone and given
            // the keyboard back.
            Task { perform(request) }
        }
    }
}
