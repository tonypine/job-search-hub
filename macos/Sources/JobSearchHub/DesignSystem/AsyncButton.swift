import SwiftUI

/// A button for work that takes a while. While it runs, the button shows its
/// busy label beside a spinner and can't be pressed again.
struct AsyncButton: View {
    let title: String
    /// What shows while it runs: "Writing…", "Reading…".
    let busyTitle: String
    var systemImage: String?
    /// Busy for a reason the button doesn't run itself, such as the same
    /// work started elsewhere.
    var isBusy = false
    let action: @MainActor () async -> Void
    @State private var isRunning = false

    init(
        _ title: String, busyTitle: String, systemImage: String? = nil, isBusy: Bool = false,
        action: @escaping @MainActor () async -> Void
    ) {
        self.title = title
        self.busyTitle = busyTitle
        self.systemImage = systemImage
        self.isBusy = isBusy
        self.action = action
    }

    var body: some View {
        Button {
            isRunning = true
            Task {
                await action()
                isRunning = false
            }
        } label: {
            if isRunning || isBusy {
                HStack(spacing: Space.xs) {
                    ProgressView().controlSize(.mini)
                    Text(busyTitle)
                }
            } else if let systemImage {
                Label(title, systemImage: systemImage)
            } else {
                Text(title)
            }
        }
        .disabled(isRunning || isBusy)
        .accessibilityValue(isRunning || isBusy ? busyTitle : "")
    }
}
