import Foundation

/// What a row of the ⌘K palette is: a kind of thing to open, a page, or an
/// action. The palette groups its rows by kind, in this order when they
/// match equally well.
public enum PaletteKind: Int, CaseIterable, Comparable, Identifiable, Sendable {
    case job, company, person, page, action

    public var id: Int { rawValue }

    /// The group's heading.
    public var title: String {
        switch self {
        case .job: "Jobs"
        case .company: "Companies"
        case .person: "People"
        case .page: "Pages"
        case .action: "Actions"
        }
    }

    /// The most rows of the kind the palette lists: enough to choose among,
    /// few enough that the actions stay in view.
    public var limit: Int {
        switch self {
        case .job: 6
        case .company, .person: 4
        case .page, .action: 12
        }
    }

    public static func < (left: PaletteKind, right: PaletteKind) -> Bool { left.rawValue < right.rawValue }
}

/// The rare and bulk actions the palette runs, which keeps them out of the
/// toolbars.
public enum PaletteAction: String, CaseIterable, Sendable {
    case addCompany
    case addCompanyFromSuggestions
    case addJobByURL
    case generateMissingCVs
    case pauseLocalModels
    case resumeLocalModels
    case markAllUpdatesSeen

    public var title: String {
        switch self {
        case .addCompany: "Add a company…"
        case .addCompanyFromSuggestions: "Add a company from suggestions…"
        case .addJobByURL: "Add a job by URL…"
        case .generateMissingCVs: "Generate missing CVs"
        case .pauseLocalModels: "Pause local models"
        case .resumeLocalModels: "Resume local models"
        case .markAllUpdatesSeen: "Mark all updates seen"
        }
    }

    public var symbolName: String {
        switch self {
        case .addCompany, .addJobByURL: "plus.circle"
        case .addCompanyFromSuggestions: "sparkles"
        case .generateMissingCVs: "doc.badge.gearshape"
        case .pauseLocalModels: "pause.circle"
        case .resumeLocalModels: "play.circle"
        case .markAllUpdatesSeen: "checkmark.circle"
        }
    }

    /// Words it's looked for by that its title doesn't say.
    var keywords: [String] {
        switch self {
        case .addCompany: ["new", "watch", "research"]
        case .addCompanyFromSuggestions: ["new", "linkedin", "startups"]
        case .addJobByURL: ["new", "posting", "link"]
        case .generateMissingCVs: ["resume", "print", "tailor"]
        case .pauseLocalModels: ["stop", "background", "work"]
        case .resumeLocalModels: ["start", "background", "work"]
        case .markAllUpdatesSeen: ["read", "clear", "unseen"]
        }
    }

    /// The actions the palette offers now: Pause or Resume as the local
    /// models stand (neither while that's unknown), and Mark all seen only
    /// with something unseen.
    public static func getAvailable(isModelWorkPaused: Bool?, unseenUpdates: Int) -> [PaletteAction] {
        allCases.filter { action in
            switch action {
            case .pauseLocalModels: isModelWorkPaused == false
            case .resumeLocalModels: isModelWorkPaused == true
            case .markAllUpdatesSeen: unseenUpdates > 0
            default: true
            }
        }
    }
}

/// What choosing a row does: open a job, company or person in the
/// inspector, switch the page, or run an action.
public enum PaletteTarget: Hashable, Sendable {
    case open(InspectorSubject)
    case page(Page)
    case action(PaletteAction)
}

/// A chip beside a row's title: a job's screen, a person's relation.
public struct PaletteChip: Hashable, Sendable {
    public var text: String
    public var tone: Tone

    public init(_ text: String, tone: Tone) {
        self.text = text
        self.tone = tone
    }
}

/// One row of the palette: its title, a line of detail, and what choosing
/// it does.
public struct PaletteItem: Hashable, Identifiable, Sendable {
    public var kind: PaletteKind
    public var target: PaletteTarget
    public var title: String
    public var detail: String?
    public var chip: PaletteChip?
    public var symbolName: String
    /// Words it's also found by, which don't show.
    public var keywords: [String]

    public var id: PaletteTarget { target }

    public init(
        kind: PaletteKind, target: PaletteTarget, title: String, detail: String? = nil, chip: PaletteChip? = nil,
        symbolName: String, keywords: [String] = []
    ) {
        self.kind = kind
        self.target = target
        self.title = title
        self.detail = detail
        self.chip = chip
        self.symbolName = symbolName
        self.keywords = keywords
    }

    /// "Senior Product Engineer", at "Northwind · Remote", with its screen.
    public static func job(_ item: JobListItem) -> PaletteItem {
        let detail = [item.companyName, item.job.location].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        return PaletteItem(
            kind: .job, target: .open(.job(item.id)), title: item.job.title, detail: detail.isEmpty ? nil : detail,
            chip: PaletteChip(item.fit.level.label, tone: item.fit.level.tone), symbolName: Page.jobs.symbolName
        )
    }

    /// "Northwind", at its domain.
    public static func company(_ summary: CompanySummary) -> PaletteItem {
        PaletteItem(
            kind: .company, target: .open(.company(summary.id)), title: summary.company.name, detail: summary.company.domain,
            symbolName: Page.companies.symbolName
        )
    }

    /// "Alex Kim", a "Connection at Northwind", found by their role too.
    public static func person(_ person: RelatedPerson) -> PaletteItem {
        let detail = person.companyName.map { "\(person.relationTitle) at \($0)" } ?? person.relationTitle
        return PaletteItem(
            kind: .person, target: .open(.person(person.reference)), title: person.name, detail: detail,
            symbolName: "person", keywords: person.role.map { [$0] } ?? []
        )
    }

    public static func page(_ page: Page) -> PaletteItem {
        PaletteItem(kind: .page, target: .page(page), title: page.title, detail: "Page", symbolName: page.symbolName)
    }

    public static func action(_ action: PaletteAction) -> PaletteItem {
        PaletteItem(kind: .action, target: .action(action), title: action.title, symbolName: action.symbolName, keywords: action.keywords)
    }
}

/// A group of the palette's rows, of one kind.
public struct PaletteSection: Equatable, Identifiable, Sendable {
    public var kind: PaletteKind
    public var items: [PaletteItem]

    public var id: PaletteKind { kind }
}

/// Finds the palette's rows for what was typed, best first.
public enum PaletteSearch {
    /// The rows for the query, grouped by kind. With nothing typed, the pages
    /// and actions to pick from. Typed, every word of it has to match the
    /// row's title, detail or keywords; rows rank by how well, and equally
    /// good ones keep the order given. The group holding the best row comes
    /// first, so Return opens it.
    public static func rank(_ items: [PaletteItem], query: String) -> [PaletteSection] {
        let tokens = tokenize(query)
        guard !tokens.isEmpty else {
            return group(items.filter { $0.kind == .page || $0.kind == .action }.map { (item: $0, score: 0) })
        }
        let phrase = normalize(query).trimmingCharacters(in: .whitespaces)
        let scored = items.compactMap { item in score(item, tokens: tokens, phrase: phrase).map { (item: item, score: $0) } }
        return group(scored)
    }

    /// How well the row matches every word typed, or nil when a word
    /// matches nothing. The title counts most, a word's start more than its
    /// middle, and the whole query as the title, or its start, most of all.
    static func score(_ item: PaletteItem, tokens: [String], phrase: String) -> Int? {
        let title = normalize(item.title)
        let titleWords = tokenize(item.title)
        let detailWords = tokenize(item.detail ?? "")
        let keywordWords = item.keywords.flatMap(tokenize)
        let initials = String(titleWords.compactMap(\.first))
        var total = 0
        for token in tokens {
            let tokenScore: Int
            if titleWords.first?.hasPrefix(token) == true {
                tokenScore = 120
            } else if titleWords.contains(where: { $0.hasPrefix(token) }) {
                tokenScore = 100
            } else if token.count > 1 && initials.hasPrefix(token) {
                tokenScore = 80
            } else if title.contains(token) {
                tokenScore = 60
            } else if detailWords.contains(where: { $0.hasPrefix(token) }) {
                tokenScore = 40
            } else if keywordWords.contains(where: { $0.hasPrefix(token) }) {
                tokenScore = 30
            } else if detailWords.contains(where: { $0.contains(token) }) {
                tokenScore = 20
            } else {
                return nil
            }
            total += tokenScore
        }
        let bareTitle = tokenize(item.title).joined(separator: " ")
        if title == phrase || bareTitle == phrase {
            total += 1000
        } else if title.hasPrefix(phrase) || bareTitle.hasPrefix(phrase) {
            total += 500
        }
        return total
    }

    /// Sorts each kind's rows by score, keeping the order given on ties, cuts
    /// them to the kind's limit, and orders the groups by their best row.
    private static func group(_ scored: [(item: PaletteItem, score: Int)]) -> [PaletteSection] {
        let indexed = scored.enumerated().map { (index: $0.offset, item: $0.element.item, score: $0.element.score) }
        let sections = PaletteKind.allCases.compactMap { kind -> (section: PaletteSection, best: Int)? in
            let rows = indexed.filter { $0.item.kind == kind }
                .sorted { $0.score != $1.score ? $0.score > $1.score : $0.index < $1.index }
                .prefix(kind.limit)
            guard let best = rows.first?.score else { return nil }
            return (PaletteSection(kind: kind, items: rows.map(\.item)), best)
        }
        return sections
            .sorted { $0.best != $1.best ? $0.best > $1.best : $0.section.kind < $1.section.kind }
            .map(\.section)
    }

    /// Lowercased, without accents: "Zürich" is found by "zurich".
    static func normalize(_ text: String) -> String {
        text.folding(options: [.caseInsensitive, .diacriticInsensitive], locale: nil)
    }

    /// The words of the text, normalized: letters and digits, split on
    /// everything else.
    static func tokenize(_ text: String) -> [String] {
        normalize(text).split { !$0.isLetter && !$0.isNumber }.map(String.init)
    }
}
