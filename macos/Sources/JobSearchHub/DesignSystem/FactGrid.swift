import SwiftUI

/// Facts as right-aligned labels beside their values. Holds `FactRow`s.
struct FactGrid<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: Space.m, verticalSpacing: Space.s) {
            content
        }
        .textSelection(.enabled)
    }
}

/// One fact: its label, and its value. A row given text leaves itself out
/// when the text is missing or empty.
struct FactRow<Value: View>: View {
    let label: String
    var help: String?
    let isShown: Bool
    @ViewBuilder let value: Value

    init(_ label: String, help: String? = nil, @ViewBuilder value: () -> Value) {
        self.label = label
        self.help = help
        self.isShown = true
        self.value = value()
    }

    var body: some View {
        if isShown {
            GridRow {
                Text(label).foregroundStyle(.secondary).gridColumnAlignment(.trailing).help(help ?? "")
                value.frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }
}

extension FactRow where Value == Text {
    init(_ label: String, text: String?, help: String? = nil) {
        self.label = label
        self.help = help
        self.isShown = !(text ?? "").isEmpty
        self.value = Text(text ?? "")
    }
}
