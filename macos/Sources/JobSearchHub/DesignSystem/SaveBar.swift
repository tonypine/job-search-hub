import JobSearchHubCore
import SwiftUI

/// A form's unsaved changes (P11): how many, with Revert and Save (⌘S). It
/// rises from the bottom once something changed and stays until the form is
/// saved or reverted.
struct SaveBar: View {
    let changeCount: Int
    let isSaving: Bool
    let revert: () -> Void
    let save: @MainActor () async -> Void

    var body: some View {
        HStack(spacing: Space.m) {
            Circle().fill(Color.hubAccent).frame(width: 7, height: 7)
                .accessibilityHidden(true)
            Text(Self.describe(changeCount))
                .fontWeight(.medium)
                .monospacedDigit()
            Spacer(minLength: Space.l)
            Button("Revert", action: revert)
                .disabled(isSaving)
                .help("Put back what was saved")
            AsyncButton("Save", busyTitle: "Saving…", isBusy: isSaving, action: save)
                .keyboardShortcut("s")
                .buttonStyle(.borderedProminent)
                .help("Save the changes (⌘S)")
        }
        .padding(.leading, Space.l)
        .padding(.trailing, Space.s)
        .padding(.vertical, Space.s)
        .frame(maxWidth: 560)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(.separator))
        .shadow(color: .black.opacity(0.1), radius: 8, y: 2)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Unsaved changes")
    }

    static func describe(_ changeCount: Int) -> String {
        changeCount == 1 ? "1 unsaved change" : "\(changeCount) unsaved changes"
    }
}

extension View {
    /// The save bar under the content while there are changes, rising as the
    /// first one is made.
    func saveBar(changeCount: Int, isSaving: Bool, revert: @escaping () -> Void, save: @escaping @MainActor () async -> Void) -> some View {
        safeAreaInset(edge: .bottom, spacing: 0) {
            if changeCount > 0 {
                SaveBar(changeCount: changeCount, isSaving: isSaving, revert: revert, save: save)
                    .padding(.horizontal, Space.l)
                    .padding(.vertical, Space.m)
                    .frame(maxWidth: .infinity)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.snappy, value: changeCount > 0)
    }
}
