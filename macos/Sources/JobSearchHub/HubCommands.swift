import SwiftUI

/// What the shown page adds, which File › Add… (⌘N) runs: the first item of
/// the page's Add menu.
struct PageAddAction {
    /// The menu item's title: "Add Job by URL…".
    let title: String
    let perform: () -> Void
}

extension FocusedValues {
    @Entry var pageAdd: PageAddAction?
}

/// The app's menu commands: the page's Add in the File menu, and Refresh in
/// the View menu for when the event stream missed something.
struct HubCommands: Commands {
    let events: HubEventStream
    @FocusedValue(\.pageAdd) private var pageAdd

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button(pageAdd?.title ?? "Add…") { pageAdd?.perform() }
                .keyboardShortcut("n")
                .disabled(pageAdd == nil)
        }
        CommandGroup(before: .toolbar) {
            Button("Refresh") { events.requestRefresh() }
                .keyboardShortcut("r")
            Divider()
        }
    }
}
