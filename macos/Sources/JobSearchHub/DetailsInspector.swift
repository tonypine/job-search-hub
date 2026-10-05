import JobSearchHubCore
import SwiftUI

/// What the window's details inspector shows, and its history. Pages put
/// their selection here, and links inside the inspector open there, so they
/// never switch the page under it. The inspector sits on the split view
/// rather than on a page: the window's toolbar then gives it a section of its
/// own, and a page's toolbar stays over the page.
@MainActor
@Observable
final class DetailsInspector {
    /// The page the history started on, so a page left behind never shows
    /// its inspector over another.
    private(set) var page: Page?
    private(set) var history = InspectorHistory()
    /// A session to resume, or start, once its Session tab shows.
    private(set) var sessionToStart: InspectorSubject?

    /// Shows a page's selection, or hides the inspector when nothing is
    /// selected. Another page than before starts a new history.
    func show(_ subject: InspectorSubject?, tab: InspectorTab? = nil, from page: Page) {
        if self.page != page {
            self.page = page
            hide()
        }
        guard let subject else {
            hide()
            return
        }
        history.push(subject, tab: tab)
    }

    /// Opens a subject from inside the inspector, as the next step of its
    /// history, over the same page.
    func open(_ subject: InspectorSubject, tab: InspectorTab? = nil) {
        history.push(subject, tab: tab)
    }

    /// Opens a subject's Session tab over the page and resumes its latest
    /// session, or starts one.
    func openSession(_ subject: InspectorSubject, from page: Page) {
        show(subject, tab: .session, from: page)
        sessionToStart = subject
    }

    func sessionStarted() {
        sessionToStart = nil
    }

    func goBack() {
        history.goBack()
    }

    func goForward() {
        history.goForward()
    }

    func select(_ tab: InspectorTab) {
        history.select(tab)
    }

    func hide() {
        history.clear()
        sessionToStart = nil
    }

    func getEntry(on page: Page) -> InspectorEntry? {
        self.page == page ? history.current : nil
    }
}

struct DetailsInspectorContent: View {
    let entry: InspectorEntry
    let client: HubClient
    @Environment(DetailsInspector.self) private var details

    var body: some View {
        // Each subject gets views of its own, so one's state never shows for another.
        Group {
            switch entry.subject {
            case let .job(jobID):
                JobDetailView(jobID: jobID, client: client, tab: tab)
            case let .company(companyID):
                CompanyInspector(companyID: companyID, client: client, tab: tab)
            case let .person(reference):
                PersonInspector(reference: reference, client: client, tab: tab)
            case .profileInterview:
                ClaudeSessionPane(subject: .profile, client: client, startsOnAppear: true, openingMessage: "Let's work on my knowledge base.")
            }
        }
        .id(entry.subject)
    }

    private var tab: Binding<InspectorTab> {
        Binding(get: { entry.tab }, set: { details.select($0) })
    }
}

/// The inspector's own controls, on a bar at its top: back and forward
/// through what it showed (⌘[ and ⌘]), and Hide. They stay out of the
/// window's toolbar: items declared inside the inspector get a section of
/// the toolbar that follows the inspector's divider, and that section could
/// keep the window's constraint updates from settling until AppKit stopped
/// the app.
struct InspectorNavigationBar: View {
    let details: DetailsInspector

    var body: some View {
        HStack(spacing: Space.xs) {
            Button("Back", systemImage: "chevron.backward") { details.goBack() }
                .keyboardShortcut("[", modifiers: .command)
                .disabled(!details.history.canGoBack)
                .help("Back (⌘[)")
            Button("Forward", systemImage: "chevron.forward") { details.goForward() }
                .keyboardShortcut("]", modifiers: .command)
                .disabled(!details.history.canGoForward)
                .help("Forward (⌘])")
            Spacer(minLength: 0)
            Button("Hide details", systemImage: "sidebar.trailing") { details.hide() }
                .help("Close the details")
        }
        .buttonStyle(.borderless)
        .labelStyle(.iconOnly)
        .padding(.horizontal, Space.m)
        .padding(.top, Space.s)
    }
}

/// One job, company or person in the inspector: its header and action bar
/// on top, its tabs, then the chosen tab, which scrolls, or which a session
/// fills to the bottom.
struct EntityInspector<Top: View, Content: View>: View {
    let tabs: [InspectorTab]
    @Binding var tab: InspectorTab
    @ViewBuilder let top: Top
    @ViewBuilder let content: (InspectorTab) -> Content

    init(
        tabs: [InspectorTab], tab: Binding<InspectorTab>, @ViewBuilder top: () -> Top,
        @ViewBuilder content: @escaping (InspectorTab) -> Content
    ) {
        self.tabs = tabs
        _tab = tab
        self.top = top()
        self.content = content
    }

    var body: some View {
        let shown = InspectorTab.resolve(tab, among: tabs)
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: Space.m) {
                top
                Picker("Show", selection: Binding(get: { shown }, set: { tab = $0 })) {
                    ForEach(tabs) { tab in Text(tab.title).tag(tab) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                // Its natural width, rather than the column's, so the control
                // never resizes as the inspector's width settles.
                .fixedSize()
            }
            .padding(.horizontal, Space.l)
            .padding(.top, Space.m)
            .padding(.bottom, Space.s)
            if shown == .session {
                Divider()
                content(shown)
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: Space.l) {
                        content(shown)
                    }
                    .padding(Space.l)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
}

/// A subject's Session tab: its Claude session, resumed or started when it
/// was opened from a session list.
struct InspectorSessionTab: View {
    let subject: InspectorSubject
    let client: HubClient
    @Environment(DetailsInspector.self) private var details

    var body: some View {
        if let session = subject.sessionSubject {
            ClaudeSessionPane(
                subject: session, client: client, startsOnAppear: details.sessionToStart == subject,
                onStartedOnAppear: { details.sessionStarted() }
            )
        }
    }
}

/// A row that opens a job, company or person in the inspector: the main
/// line, a secondary one, chips, and a chevron.
struct InspectorLinkRow<Chips: View>: View {
    let title: String
    var detail: String?
    @ViewBuilder let chips: Chips
    let open: () -> Void

    init(_ title: String, detail: String? = nil, @ViewBuilder chips: () -> Chips, open: @escaping () -> Void) {
        self.title = title
        self.detail = detail
        self.chips = chips()
        self.open = open
    }

    var body: some View {
        Button(action: open) {
            HStack(alignment: .center, spacing: Space.s) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).fontWeight(.semibold).multilineTextAlignment(.leading)
                    if let detail, !detail.isEmpty {
                        Text(detail).font(.hubSecondary).foregroundStyle(.secondary)
                    }
                    if Chips.self != EmptyView.self {
                        HStack(spacing: Space.xs) { chips }.padding(.top, 2)
                    }
                }
                Spacer(minLength: 0)
                Image(systemName: "chevron.forward").foregroundStyle(.tertiary)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}
