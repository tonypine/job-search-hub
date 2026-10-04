import Foundation

/// What the window's inspector shows: a job, a company or a person, or the
/// interview that deepens the owner's knowledge base.
public enum InspectorSubject: Hashable, Sendable {
    case job(UUID)
    case company(UUID)
    /// A recruiter, by the LinkedIn conversation they started.
    case person(UUID)
    case profileInterview

    /// The subject's Claude session, for the kinds that have one.
    public var sessionSubject: ClaudeSessionSubject? {
        switch self {
        case let .job(id): .job(id)
        case let .company(id): .company(id)
        case .profileInterview: .profile
        case .person: nil
        }
    }

    public init(_ session: ClaudeSessionSubject) {
        switch session {
        case let .job(id): self = .job(id)
        case let .company(id): self = .company(id)
        case .profile: self = .profileInterview
        }
    }
}

/// A tab of the inspector. A kind has the same tabs everywhere.
public enum InspectorTab: String, CaseIterable, Identifiable, Sendable {
    case overview, prep, posting, jobs, people, conversation, session

    public var id: String { rawValue }
    public var title: String { rawValue.capitalized }

    /// The subject's tabs, in order. A job has Prep once it's pursued or has
    /// a CV; the profile interview is its session alone.
    public static func getTabs(for subject: InspectorSubject, hasPrep: Bool = false) -> [InspectorTab] {
        switch subject {
        case .job: hasPrep ? [.overview, .prep, .posting, .session] : [.overview, .posting, .session]
        case .company: [.overview, .jobs, .people, .session]
        case .person: [.overview, .conversation]
        case .profileInterview: [.session]
        }
    }

    /// The tab to show: the one asked for when the subject has it, or its
    /// first.
    public static func resolve(_ tab: InspectorTab, among tabs: [InspectorTab]) -> InspectorTab {
        tabs.contains(tab) ? tab : tabs.first ?? .overview
    }
}

/// One step of the inspector's history: what it showed, on which tab.
public struct InspectorEntry: Equatable, Sendable {
    public var subject: InspectorSubject
    public var tab: InspectorTab

    public init(_ subject: InspectorSubject, tab: InspectorTab = .overview) {
        self.subject = subject
        self.tab = tab
    }
}

/// Back and forward through what the inspector showed, like a browser's
/// history: opening something drops what lay ahead.
public struct InspectorHistory: Equatable, Sendable {
    /// The most steps kept; the oldest go first.
    public static let limit = 100

    public private(set) var entries: [InspectorEntry] = []
    /// Where in `entries` the inspector is; nil while empty.
    public private(set) var position: Int?

    public init() {}

    public var current: InspectorEntry? { position.map { entries[$0] } }
    public var canGoBack: Bool { (position ?? 0) > 0 }
    public var canGoForward: Bool { position.map { $0 < entries.count - 1 } ?? false }

    /// Shows a subject as the next step. The subject already shown stays one
    /// step, moved to the tab asked for, or kept on its own without one.
    public mutating func push(_ subject: InspectorSubject, tab: InspectorTab? = nil) {
        if let position, entries[position].subject == subject {
            if let tab { entries[position].tab = tab }
            return
        }
        let kept = position.map { Array(entries[...$0]) } ?? []
        entries = Array((kept + [InspectorEntry(subject, tab: tab ?? .overview)]).suffix(Self.limit))
        position = entries.count - 1
    }

    public mutating func goBack() {
        guard canGoBack, let position else { return }
        self.position = position - 1
    }

    public mutating func goForward() {
        guard canGoForward, let position else { return }
        self.position = position + 1
    }

    /// Switches the shown step's tab, so going back returns to it.
    public mutating func select(_ tab: InspectorTab) {
        guard let position else { return }
        entries[position].tab = tab
    }

    public mutating func clear() {
        entries = []
        position = nil
    }
}
