import Foundation

/// The Companies page's scopes: the watch list, the companies with open jobs
/// that pass the screen, and the ones the hub suggests researching.
public enum CompanyScope: String, CaseIterable, Codable, Sendable {
    case watching
    case withOpenJobs
    case suggested

    public var title: String {
        switch self {
        case .watching: "Watching"
        case .withOpenJobs: "With open jobs"
        case .suggested: "Suggested"
        }
    }

    /// The stored companies the scope lists; Suggested lists suggestions,
    /// which the hub doesn't hold, instead.
    public func getSummaries(_ summaries: [CompanySummary]) -> [CompanySummary] {
        switch self {
        case .watching: summaries.filter { $0.watchedSince != nil }
        case .withOpenJobs: summaries.filter(\.hasFittingJobs)
        case .suggested: []
        }
    }
}

/// Where the owner stands with a company, in its row's *You* column.
public enum CompanyStanding: Equatable, Sendable {
    /// The phase of the open application updated last: "Applied".
    case phase(String)
    /// No application, and a job that passes the screen appeared today.
    case newJobToday

    public var text: String {
        switch self {
        case let .phase(name): name
        case .newJobToday: "New job today"
        }
    }
}

public extension CompanySummary {
    /// What the row says under the name: "Canada · 51-200 people", or the
    /// domain when the hub knows neither.
    var contextLine: String {
        var parts: [String] = []
        if let country = company.headquartersCountry, !country.isEmpty {
            parts.append(country)
        }
        if let size = company.employeeCountRange, !size.isEmpty {
            parts.append("\(size) people")
        }
        return parts.isEmpty ? company.domain : parts.joined(separator: " · ")
    }

    var hasFittingJobs: Bool { (fittingJobs ?? 0) > 0 }

    /// "3 open", or "None that pass".
    var openJobsText: String {
        hasFittingJobs ? "\(fittingJobs ?? 0) open" : "None that pass"
    }

    /// The open application's phase, or *New job today* when a job that
    /// passes the screen was first seen today; nil otherwise.
    func getStanding(now: Date, calendar: Calendar = .current) -> CompanyStanding? {
        if let phase = applicationPhase, !phase.isEmpty {
            return .phase(phase)
        }
        if let seenAt = newestFittingJobSeenAt, calendar.isDate(seenAt, inSameDayAs: now) {
            return .newJobToday
        }
        return nil
    }

    /// The provider of the company's first board as people write it,
    /// "Greenhouse"; nil when the hub found no board.
    var boardName: String? {
        jobBoards.first.map { CompanyBoards.getProviderName($0.provider) }
    }

    /// Whether `target`, what a research was started on (a name, a domain or
    /// a link), is this company.
    func isResearched(as target: String) -> Bool {
        let wanted = target.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !wanted.isEmpty else { return false }
        let domain = company.domain.lowercased()
        return company.name.lowercased() == wanted || (!domain.isEmpty && (wanted == domain || CompanyBoards.getHost(wanted) == domain))
    }
}

public extension CompanySuggestion {
    /// Whether `target`, what a research was started on, is this suggestion.
    func isResearched(as target: String) -> Bool {
        let wanted = target.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return !wanted.isEmpty && (researchTarget.lowercased() == wanted || organization.lowercased() == wanted)
    }
}

public enum CompanyBoards {
    /// A board provider as people write it: "smartrecruiters" is
    /// "SmartRecruiters", "greenhouse" is "Greenhouse".
    public static func getProviderName(_ provider: String) -> String {
        switch provider.lowercased() {
        case "smartrecruiters": "SmartRecruiters"
        case "bamboohr": "BambooHR"
        default: provider.prefix(1).uppercased() + provider.dropFirst()
        }
    }

    /// A link's host without "www.", or nil for text that isn't a link.
    static func getHost(_ text: String) -> String? {
        guard text.contains("/"), let host = URL(string: text.hasPrefix("http") ? text : "https://" + text)?.host() else { return nil }
        return host.hasPrefix("www.") ? String(host.dropFirst(4)) : host
    }
}

/// The step a company's research is on, read from the lines `hub company
/// add` prints: one per tool the agent calls, as "  · WebFetch {…}".
public struct ResearchStep: Equatable, Sendable {
    /// What the agent is doing, "reading acme.example".
    public var text: String
    /// How many tools it has called, this one included; 0 before the first.
    public var number: Int

    public init(text: String, number: Int) {
        self.text = text
        self.number = number
    }

    /// The step the lines end on: "starting" before the agent calls a tool.
    public init(lines: [String]) {
        let calls = lines.compactMap(Self.parseCall)
        guard let last = calls.last else {
            self.init(text: "starting", number: 0)
            return
        }
        self.init(text: Self.describe(tool: last.tool, input: last.input), number: calls.count)
    }

    /// "Researching: reading acme.example · step 3".
    public var line: String {
        number == 0 ? "Researching: \(text)" : "Researching: \(text) · step \(number)"
    }

    /// The tool and its input from a line "  · <tool> <input>".
    static func parseCall(_ line: String) -> (tool: String, input: String)? {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        guard trimmed.hasPrefix("· ") else { return nil }
        let call = trimmed.dropFirst(2)
        let tool = call.prefix { !$0.isWhitespace }
        guard !tool.isEmpty else { return nil }
        return (String(tool), String(call.dropFirst(tool.count)).trimmingCharacters(in: .whitespaces))
    }

    static func describe(tool: String, input: String) -> String {
        switch tool {
        case "WebSearch": return "searching the web"
        case "WebFetch":
            guard let url = input.firstMatch(of: #/"url"\s*:\s*"([^"]+)"/#), let host = CompanyBoards.getHost(String(url.output.1)) else {
                return "reading a page"
            }
            return "reading \(host)"
        case "find_companies", "get_company", "list_watch_list": return "checking what the hub holds"
        case "create_company", "update_company": return "saving the company"
        case "set_job_board": return "setting its job board"
        case "add_person": return "noting who works there"
        case "add_to_watch_list": return "adding it to the watch list"
        default: return tool.replacingOccurrences(of: "_", with: " ")
        }
    }
}
