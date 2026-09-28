/// The app's top-level pages, in sidebar order.
public enum Page: String, CaseIterable, Identifiable, Sendable {
    case pipeline
    case jobs
    case companies
    case profile
    case settings

    public var id: String { rawValue }

    public var title: String {
        switch self {
        case .pipeline: "Pipeline"
        case .jobs: "Jobs"
        case .companies: "Companies"
        case .profile: "Profile"
        case .settings: "Settings"
        }
    }

    /// The SF Symbol shown beside the title in the sidebar.
    public var symbolName: String {
        switch self {
        case .pipeline: "rectangle.split.3x1"
        case .jobs: "briefcase"
        case .companies: "building.2"
        case .profile: "person.crop.circle"
        case .settings: "gearshape"
        }
    }
}
