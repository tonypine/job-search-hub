/// The app's top-level pages, in sidebar order.
public enum Page: String, CaseIterable, Identifiable, Sendable {
    case pipeline
    case updates
    case jobs
    case companies
    case recruiters
    case profile
    case prompts
    case settings

    public var id: String { rawValue }

    public var title: String {
        switch self {
        case .pipeline: "Pipeline"
        case .updates: "Updates"
        case .jobs: "Jobs"
        case .companies: "Companies"
        case .recruiters: "Recruiters"
        case .profile: "Profile"
        case .prompts: "Prompts"
        case .settings: "Settings"
        }
    }

    /// The SF Symbol shown beside the title in the sidebar.
    public var symbolName: String {
        switch self {
        case .pipeline: "rectangle.split.3x1"
        case .updates: "bell"
        case .jobs: "briefcase"
        case .companies: "building.2"
        case .recruiters: "person.crop.rectangle.stack"
        case .profile: "person.crop.circle"
        case .prompts: "text.bubble"
        case .settings: "gearshape"
        }
    }
}
