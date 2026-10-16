import JobSearchHubCore
import Observation

/// The edits not saved yet across the app's pages, which an install waits
/// for: each page with an editor says when it holds changes.
@MainActor
@Observable
final class UnsavedEdits {
    static let shared = UnsavedEdits()

    private struct Entry {
        let edit: UnsavedEdit
        let page: Page
    }

    private var entries: [String: Entry] = [:]

    /// Records whether the editor `id`, on `page`, holds unsaved changes.
    func set(_ id: String, title: String, page: Page, isUnsaved: Bool) {
        if isUnsaved {
            entries[id] = Entry(edit: UnsavedEdit(id: id, title: title), page: page)
        } else if entries[id] != nil {
            entries[id] = nil
        }
    }

    var list: [UnsavedEdit] {
        entries.values.map(\.edit).sorted { $0.title < $1.title }
    }

    /// The page the edit is on, which its line in the install sheet opens.
    func getPage(of id: String) -> Page? {
        entries[id]?.page
    }
}
