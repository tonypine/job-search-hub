import AppKit
import JobSearchHubCore
import SwiftTerm
import SwiftUI

/// A job opened as a page in the content column, in place of the list it
/// came from (P2): a top bar to go back, where the job stands in the list
/// and ↑ ↓ through it, then the job at a page's width (see JobDetailView's
/// page layout). Esc or ⌘[ goes back to the list, where the job is selected
/// and the list scrolled as it was.
struct JobPageView: View {
    let jobID: UUID
    /// The page whose list the job was opened from.
    let listPage: Page
    let client: HubClient
    let goBack: () -> Void
    @Environment(DetailsInspector.self) private var details
    @State private var title: String?

    var body: some View {
        VStack(spacing: 0) {
            topBar
            Divider()
            JobDetailView(jobID: jobID, client: client, tab: tab, layout: .page) { title = $0 }
                .id(jobID)
        }
        // The column's width is the window's to give: the page asks for none
        // of its own, so neither of its layouts can change the window's
        // minimum size as it switches between them.
        .frame(minWidth: 0, maxWidth: .infinity, minHeight: 0, maxHeight: .infinity, alignment: .top)
        .background(.background)
        // ↑ ↓, Esc and ⌘[ are the page's; letters go on to the job's own
        // keys (see JobDetailView).
        .background {
            JobPageKeys { key in
                switch key {
                // At either end the key still stops here, out of the hidden
                // list's reach.
                case .previous:
                    move(by: -1)
                    return true
                case .next:
                    move(by: 1)
                    return true
                case .back:
                    goBack()
                    return true
                case .letter: return false
                }
            }
        }
    }

    /// "‹ Jobs / Senior Product Engineer", then "3 of 48", ↑ and ↓, and
    /// collapse back to the list and the inspector.
    private var topBar: some View {
        let position = details.getPosition(on: listPage)
        return HStack(spacing: Space.s) {
            Button(action: goBack) {
                Label(listPage.title, systemImage: "chevron.backward")
                    .labelStyle(.titleAndIcon)
            }
            .buttonStyle(.link)
            .help("Back to \(listPage.title) (Esc or ⌘[)")
            Text("/").foregroundStyle(.tertiary).accessibilityHidden(true)
            Text(title ?? "")
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.tail)
            Spacer(minLength: Space.m)
            if let position {
                Text(position)
                    .font(.hubCaption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                    .fixedSize()
            }
            Group {
                Button("Previous job", systemImage: "chevron.up") { move(by: -1) }
                    .disabled(details.getNeighbor(by: -1, on: listPage) == nil)
                    .help("The list's previous job (↑)")
                Button("Next job", systemImage: "chevron.down") { move(by: 1) }
                    .disabled(details.getNeighbor(by: 1, on: listPage) == nil)
                    .help("The list's next job (↓)")
                Button("Back to the list and details", systemImage: "arrow.down.right.and.arrow.up.left", action: goBack)
                    .help("Back to the list, with the job in the details beside it")
            }
            .buttonStyle(.borderless)
            .labelStyle(.iconOnly)
        }
        .font(.hubSecondary)
        .padding(.horizontal, Space.l)
        .frame(height: 36)
    }

    private var tab: Binding<InspectorTab> {
        Binding(get: { details.history.current?.tab ?? .posting }, set: { details.select($0) })
    }

    @discardableResult
    private func move(by step: Int) -> Bool {
        guard details.getNeighbor(by: step, on: listPage) != nil else { return false }
        details.move(by: step, on: listPage)
        return true
    }
}

/// What a key does on a job's page: ↑ and ↓ move through the list, Esc
/// and ⌘[ go back to it, and a plain letter goes to the job, where P, L and
/// S decide.
enum JobPageKey: Equatable {
    case previous
    case next
    case back
    case letter(Character)

    private static let escape: UInt16 = 53
    private static let downArrow: UInt16 = 125
    private static let upArrow: UInt16 = 126

    /// The key a key-down event makes on the page; nil for any other. While
    /// a field or a session's terminal has the keyboard (`isTyping`), only
    /// ⌘[ is the page's: the rest are the field's or the terminal's.
    static func get(keyCode: UInt16, characters: String?, modifiers: NSEvent.ModifierFlags, isTyping: Bool) -> JobPageKey? {
        let modifiers = modifiers.intersection([.command, .control, .option, .shift])
        if modifiers == .command, characters == "[" { return .back }
        guard !isTyping else { return nil }
        if modifiers.isEmpty {
            switch keyCode {
            case upArrow: return .previous
            case downArrow: return .next
            case escape: return .back
            default: break
            }
        }
        guard modifiers.isSubset(of: .shift), let character = characters?.first, characters?.count == 1, character.isLetter
        else { return nil }
        return .letter(character)
    }
}

/// Hands the page's keys to `handle` while the view shows, whatever has the
/// keyboard in its window: a list hidden underneath, the window itself or
/// a button on the page. SwiftUI focus on the page never took the keyboard
/// reliably, so the keys come from the window's events, before any view
/// sees them. `handle` returns false for a key that does nothing, which then
/// goes on as usual.
struct JobPageKeys: NSViewRepresentable {
    let handle: (JobPageKey) -> Bool

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeNSView(context: Context) -> WindowView {
        let view = WindowView()
        view.coordinator = context.coordinator
        return view
    }

    func updateNSView(_ view: WindowView, context: Context) {
        context.coordinator.handle = handle
    }

    static func dismantleNSView(_ view: WindowView, coordinator: Coordinator) {
        coordinator.stop()
    }

    @MainActor final class Coordinator {
        var handle: ((JobPageKey) -> Bool)?
        private var monitor: Any?
        private weak var window: NSWindow?

        func watch(_ window: NSWindow?) {
            stop()
            guard let window else { return }
            self.window = window
            monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
                let isTaken = MainActor.assumeIsolated { self?.take(event) == true }
                return isTaken ? nil : event
            }
        }

        func stop() {
            if let monitor { NSEvent.removeMonitor(monitor) }
            monitor = nil
        }

        /// Only the page's own window, with no sheet or alert over it.
        private func take(_ event: NSEvent) -> Bool {
            guard let window, event.window === window, window.isKeyWindow, window.attachedSheet == nil,
                  let key = JobPageKey.get(
                      keyCode: event.keyCode, characters: event.charactersIgnoringModifiers, modifiers: event.modifierFlags,
                      isTyping: Self.isTyping(in: window)
                  )
            else { return false }
            return handle?(key) ?? false
        }

        /// A text field or editor, or a session's terminal, has the keyboard.
        private static func isTyping(in window: NSWindow) -> Bool {
            window.firstResponder is NSText || window.firstResponder is TerminalView
        }
    }

    final class WindowView: NSView {
        weak var coordinator: Coordinator?

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            coordinator?.watch(window)
        }
    }
}
