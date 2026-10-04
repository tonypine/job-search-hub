/// The app's top-level pages, in sidebar order. Settings is the app's
/// Settings window (⌘,), not a page.
public enum Page: String, CaseIterable, Identifiable, Sendable {
    case today
    case decide
    case pipeline
    case updates
    case jobs
    case companies
    case recruiters
    case profile
    case criteria
    case activity
    case prompts
    case modelLab = "model-lab"

    public var id: String { rawValue }

    public var title: String {
        switch self {
        case .today: "Today"
        case .decide: "Decide"
        case .pipeline: "Pipeline"
        case .updates: "Updates"
        case .jobs: "Jobs"
        case .companies: "Companies"
        case .recruiters: "Recruiters"
        case .profile: "Profile"
        case .criteria: "Criteria"
        case .activity: "Activity"
        case .prompts: "Prompts"
        case .modelLab: "Model lab"
        }
    }

    /// The SF Symbol shown beside the title in the sidebar.
    public var symbolName: String {
        switch self {
        case .today: "sun.max"
        case .decide: "checklist"
        case .pipeline: "rectangle.split.3x1"
        case .updates: "bell"
        case .jobs: "briefcase"
        case .companies: "building.2"
        case .recruiters: "person.crop.rectangle.stack"
        case .profile: "person.crop.circle"
        case .criteria: "slider.horizontal.3"
        case .activity: "gauge.with.needle"
        case .prompts: "text.bubble"
        case .modelLab: "square.split.2x1"
        }
    }

    /// Updates folded into Today, which shows the unseen ones; its full
    /// history is a page Today links to, not a sidebar row.
    public var isInSidebar: Bool { self != .updates }

    public var group: SidebarGroup {
        switch self {
        case .today, .decide, .pipeline, .updates: .work
        case .jobs, .companies, .recruiters: .browse
        case .profile, .criteria: .you
        case .activity, .prompts, .modelLab: .hub
        }
    }
}

/// The sidebar's groups, by intent: what needs you, what you browse, who you
/// are, and the hub's own tools.
public enum SidebarGroup: String, CaseIterable, Identifiable, Sendable {
    case work
    case browse
    case you
    case hub

    public var id: String { rawValue }

    /// The section's header; the first group has none.
    public var title: String? {
        switch self {
        case .work: nil
        case .browse: "Browse"
        case .you: "You"
        case .hub: "Hub"
        }
    }

    /// The hub's maintenance tools start folded away.
    public var isExpandedByDefault: Bool { self != .hub }

    /// The group's rows in the sidebar.
    public var pages: [Page] { Page.allCases.filter { $0.group == self && $0.isInSidebar } }
}
