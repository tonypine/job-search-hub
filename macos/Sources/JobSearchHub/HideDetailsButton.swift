import SwiftUI

/// The details inspector's own toolbar item. Declaring an item inside the
/// inspector gives it its own section of the window's toolbar, so the page's
/// toolbar and search field stay over the page instead of reaching over the
/// details.
struct HideDetailsButton: ToolbarContent {
    let hide: () -> Void

    var body: some ToolbarContent {
        ToolbarItem {
            Button("Hide details", systemImage: "sidebar.trailing", action: hide)
                .help("Close the details")
        }
    }
}
