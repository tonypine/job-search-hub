import JobSearchHubCore
import SwiftUI

/// A second thing a toast offers after Undo, such as "Add reason".
struct ToastAction {
    let title: String
    let perform: @MainActor () -> Void
}

/// One transient confirmation: "Skipped 3 jobs", with Undo where it can.
struct ToastMessage: Identifiable, Equatable {
    let id = UUID()
    let text: String
    var tone = Tone.positive
    var symbol = "checkmark.circle.fill"
    /// Takes the action back; nil when it can't be.
    var undo: (@MainActor () -> Void)?
    var action: ToastAction?

    static func == (left: ToastMessage, right: ToastMessage) -> Bool { left.id == right.id }
}

struct Toast: View {
    let message: ToastMessage
    let onClose: () -> Void

    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: message.symbol).foregroundStyle(message.tone.color).accessibilityHidden(true)
            Text(message.text)
            if let undo = message.undo {
                Button("Undo") {
                    undo()
                    onClose()
                }
                .buttonStyle(.link)
                .fontWeight(.semibold)
            }
            if let action = message.action {
                Button(action.title) {
                    action.perform()
                    onClose()
                }
                .buttonStyle(.link)
            }
        }
        .padding(.horizontal, Space.l)
        .padding(.vertical, Space.s)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(.separator))
        .shadow(color: .black.opacity(0.08), radius: 6, y: 2)
        .accessibilityElement(children: .combine)
    }
}

extension View {
    /// Shows the message at the bottom for a few seconds, then clears it.
    func toast(_ message: Binding<ToastMessage?>) -> some View {
        overlay(alignment: .bottom) {
            if let shown = message.wrappedValue {
                Toast(message: shown) { message.wrappedValue = nil }
                    .padding(.bottom, Space.l)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .task(id: shown.id) {
                        try? await Task.sleep(for: .seconds(shown.undo == nil && shown.action == nil ? 4 : 8))
                        if message.wrappedValue?.id == shown.id { message.wrappedValue = nil }
                    }
            }
        }
        .animation(.default, value: message.wrappedValue)
    }
}
