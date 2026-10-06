import SwiftUI

/// A page's own controls, on a bar at its top, over a divider. They stay out
/// of the window's toolbar: with the details inspector open, that toolbar
/// runs over the inspector's column too, and a page's items there sit over
/// the details. The inspector can't take a toolbar section of its own (see
/// InspectorNavigationBar), so the page keeps its controls.
///
/// The bar asks for no minimum width and clips what doesn't fit, so a narrow
/// page never pushes on the window's size, and its height stays the same
/// whatever the page's width.
struct PageBar<Content: View>: View {
    @ViewBuilder let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: Space.s) {
                Spacer(minLength: 0)
                content
            }
            .padding(.horizontal, Space.m)
            .padding(.vertical, Space.s)
            .frame(minWidth: 0, maxWidth: .infinity, alignment: .trailing)
            .clipped()
            Divider()
        }
    }
}
