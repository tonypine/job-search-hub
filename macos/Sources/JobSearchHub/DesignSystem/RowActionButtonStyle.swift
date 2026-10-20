import SwiftUI

/// A row's own action, such as a job row's Pursue, Later or Skip: its symbol
/// in a small key, as the redesign's rows draw them. Three small bordered
/// buttons took about 108 pt, and beside the inspector that was the room the
/// row's title needs (TP-1123).
struct RowActionButtonStyle: ButtonStyle {
    static let size = CGSize(width: 26, height: 22)

    func makeBody(configuration: Configuration) -> some View {
        let shape = RoundedRectangle(cornerRadius: Radius.control)
        configuration.label
            .font(.subheadline.weight(.medium))
            .foregroundStyle(.primary)
            .frame(width: Self.size.width, height: Self.size.height)
            .background(configuration.isPressed ? AnyShapeStyle(.quaternary) : AnyShapeStyle(.background), in: shape)
            .overlay(shape.strokeBorder(.separator))
            .contentShape(shape)
    }
}

extension ButtonStyle where Self == RowActionButtonStyle {
    /// A row's own action in a small key.
    static var rowAction: RowActionButtonStyle {
        RowActionButtonStyle()
    }
}
