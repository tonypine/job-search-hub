import AppKit
import JobSearchHubCore
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
        // The job takes the keyboard (see PageKeys); ↑ ↓ and Esc reach here
        // from it.
        .onKeyPress(.upArrow) { PageKeys.isTyping ? .ignored : move(by: -1) }
        .onKeyPress(.downArrow) { PageKeys.isTyping ? .ignored : move(by: 1) }
        .onExitCommand { goBack() }
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
            .keyboardShortcut("[", modifiers: .command)
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
                Button("Previous job", systemImage: "chevron.up") { _ = move(by: -1) }
                    .disabled(details.getNeighbor(by: -1, on: listPage) == nil)
                    .help("The list's previous job (↑)")
                Button("Next job", systemImage: "chevron.down") { _ = move(by: 1) }
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

    private func move(by step: Int) -> KeyPress.Result {
        guard details.getNeighbor(by: step, on: listPage) != nil else { return .ignored }
        details.move(by: step, on: listPage)
        return .handled
    }
}

/// A job's page has the keyboard as it shows: the job's view takes the
/// focus, so ↑ ↓ and Esc reach the page around it, and P, L and S reach the
/// job, which decides. Typing in a field never decides; a session's
/// terminal keeps its own keys.
struct PageKeys: ViewModifier {
    let isEnabled: Bool
    var isFocused: FocusState<Bool>.Binding
    /// Handles a letter key; false when it does nothing.
    let press: (Character) -> Bool

    /// A text field or editor has the keyboard.
    static var isTyping: Bool {
        NSApp.keyWindow?.firstResponder is NSText
    }

    func body(content: Content) -> some View {
        if isEnabled {
            content
                .focusable()
                .focused(isFocused)
                .focusEffectDisabled()
                .onAppear { isFocused.wrappedValue = true }
                .onKeyPress(characters: .letters, phases: .down) { keyPress in
                    guard keyPress.modifiers.isDisjoint(with: [.command, .control, .option]), !Self.isTyping,
                          let key = keyPress.characters.first, press(key)
                    else { return .ignored }
                    return .handled
                }
        } else {
            content
        }
    }
}
