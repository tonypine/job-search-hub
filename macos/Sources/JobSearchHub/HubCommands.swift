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
    /// Whether the window's ⌘K palette shows, which Go › Jump to… toggles.
    /// A binding to the window's state stays the same from one render to the
    /// next, unlike a closure, so the toolbar doesn't update on every render.
    @Entry var isShowingPalette: Binding<Bool>?
}

/// The app's menu commands: Check for New Version… in the app menu, the
/// page's Add in the File menu, Refresh in the View menu for when the event
/// stream missed something, and Jump to… (⌘K) in the Go menu.
struct HubCommands: Commands {
    let events: HubEventStream
    let newVersions: NewVersionChecker
    @FocusedValue(\.pageAdd) private var pageAdd
    @FocusedBinding(\.isShowingPalette) private var isShowingPalette
    @Environment(\.openSettings) private var openSettings
    @AppStorage(SettingsTab.storageKey) private var settingsTab = SettingsTab.connection

    var body: some Commands {
        // Checks right away, and opens Settings › Version, which says
        // "Checking…", then what it found.
        CommandGroup(after: .appInfo) {
            Button("Check for New Version…") {
                newVersions.checkNow()
                settingsTab = .version
                openSettings()
            }
        }
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
        CommandMenu("Go") {
            Button("Jump to…") { isShowingPalette?.toggle() }
                .keyboardShortcut("k")
                .disabled(isShowingPalette == nil)
        }
    }
}
