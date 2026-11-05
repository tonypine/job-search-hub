import SwiftUI

/// The split view's detail column: the window's banner, when there is one,
/// above the page. The page gets what the banner leaves, and the column takes
/// the height it's offered whatever the two ask for. A column that asks for
/// more grows the split view past the window, which cuts off the foot of the
/// sidebar beside it.
struct DetailColumn<Banner: View, Content: View>: View {
    @ViewBuilder let banner: Banner
    @ViewBuilder let content: Content

    var body: some View {
        VStack(spacing: 0) {
            banner
            content
                .frame(maxWidth: .infinity, minHeight: 0, maxHeight: .infinity, alignment: .top)
        }
        .frame(minHeight: 0, idealHeight: 600, maxHeight: .infinity, alignment: .top)
    }
}
