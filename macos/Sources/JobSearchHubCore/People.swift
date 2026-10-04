import Foundation

/// How a person can get the owner in.
public enum PersonRelation: String, CaseIterable, Codable, Identifiable, Sendable {
    /// Found at a company by an agent, in its dossier.
    case contact
    /// A LinkedIn connection who works at a hub company.
    case connection
    /// Someone elsewhere who can introduce you: a warm path.
    case introducer
    /// Someone who wrote on LinkedIn to recruit you.
    case recruiter

    public var id: String { rawValue }
    public var title: String { rawValue.capitalized }
    /// The filter's label: "Contacts".
    public var pluralTitle: String { title + "s" }
}

/// One person who can get the owner in, whatever the hub knows them from,
/// and what their company has open now. An introducer linked to two
/// companies is two people, one at each.
public struct RelatedPerson: Decodable, Equatable, Identifiable, Sendable {
    /// Tells people apart across relations, e.g. "recruiter:<id>".
    public var key: String
    public var relation: PersonRelation
    /// Their id where the relation keeps them; a recruiter's is the
    /// conversation they started.
    public var personID: UUID
    public var name: String
    public var role: String?
    public var companyID: UUID?
    public var companyName: String?
    public var profileURL: String?
    public var email: String?
    /// The page that named a contact.
    public var sourceURL: String?
    /// How they can help: a contact's notes, an introducer's note on the
    /// company, or why a conversation was read as a recruiter's.
    public var note: String?
    public var relevance: String?
    public var closeness: String?
    public var howKnown: String?
    public var preferredChannel: String?
    public var hiringRole: String?
    public var isAgency: Bool
    public var lastContactAt: Date?
    /// Whether the owner wrote back to a conversation they started; nil for
    /// anyone who didn't write.
    public var answered: Bool?
    public var openJobs: Int
    public var fittingJobs: Int

    public var id: String { key }
    public var reference: PersonReference { PersonReference(key: key, companyID: companyID) }

    /// Their company has open jobs in the feed now.
    public var isHiringNow: Bool { openJobs > 0 }
    /// They wrote and the owner never answered.
    public var isUnanswered: Bool { answered == false }
    /// The recruiter's conversation, for its messages and a drafted reply.
    public var conversationID: UUID? { relation == .recruiter ? personID : nil }

    /// The relation chip: "Recruiter", or "Agency recruiter".
    public var relationTitle: String { relation == .recruiter && isAgency ? "Agency recruiter" : relation.title }

    /// "Hiring manager", from a contact's relevance; nil for "other".
    public var relevanceTitle: String? {
        guard let relevance, !relevance.isEmpty, relevance != "other" else { return nil }
        return relevance.replacingOccurrences(of: "_", with: " ").capitalizingFirstLetter()
    }

    /// "3 open, 1 passes the screen", or empty.
    public var openingsText: String {
        guard openJobs > 0 else { return "" }
        guard fittingJobs > 0 else { return "\(openJobs) open" }
        return "\(openJobs) open, \(fittingJobs) " + (fittingJobs == 1 ? "passes" : "pass") + " the screen"
    }

    /// What they can do for the owner, in a sentence or two.
    public var whatTheyCanDo: String {
        let at = companyName.map { " at \($0)" } ?? ""
        let base: String
        switch relation {
        case .recruiter:
            let role = hiringRole.flatMap { $0.isEmpty ? nil : " The role: \($0)." } ?? ""
            base = (isAgency ? "Recruits through an agency" : "Recruits") + (companyName.map { " for \($0)" } ?? "") + "." + role
        case .connection:
            base = "A LinkedIn connection\(at)" + (closeness.map { ": \($0)." } ?? ".")
        case .introducer:
            base = "Can introduce you\(at)" + (note.flatMap { $0.isEmpty ? nil : ": \($0)." } ?? ".")
        case .contact:
            base = (relevanceTitle ?? "Works") + at + ", found by company research."
        }
        guard openJobs > 0, companyName != nil else { return base }
        let open = openJobs == 1 ? "1 open job" : "\(openJobs) open jobs"
        let fits = fittingJobs == 0 ? "" : fittingJobs == 1 ? ", and 1 fits you" : ", and \(fittingJobs) fit you"
        return base + " Their company has \(open)\(fits)."
    }

    enum CodingKeys: String, CodingKey {
        case key, relation, name, role, companyName, email, note, relevance, closeness, howKnown, preferredChannel, hiringRole
        case isAgency, lastContactAt, answered, openJobs, fittingJobs
        case personID = "id"
        case companyID = "companyId"
        case profileURL = "profileUrl"
        case sourceURL = "sourceUrl"
    }
}

public struct PeopleResponse: Decodable, Sendable {
    public var people: [RelatedPerson]
}

/// A person the inspector shows: their key, and their company, which
/// narrows the list it reads them from.
public struct PersonReference: Hashable, Sendable {
    public var key: String
    public var companyID: UUID?

    public init(key: String, companyID: UUID? = nil) {
        self.key = key
        self.companyID = companyID
    }
}

public enum PeopleQuery {
    /// The query for everyone, or the people at one company.
    public static func make(companyID: UUID?) -> [URLQueryItem] {
        companyID.map { [URLQueryItem(name: "company_id", value: $0.uuidString)] } ?? []
    }
}

/// Which people the list shows.
public struct PeopleFilter: Equatable, Sendable {
    /// One relation, or every one when nil.
    public var relation: PersonRelation?
    public var hiringNowOnly = false
    public var unansweredOnly = false

    public init(relation: PersonRelation? = nil, hiringNowOnly: Bool = false, unansweredOnly: Bool = false) {
        self.relation = relation
        self.hiringNowOnly = hiringNowOnly
        self.unansweredOnly = unansweredOnly
    }

    public func apply(to people: [RelatedPerson]) -> [RelatedPerson] {
        people.filter { person in
            (relation == nil || person.relation == relation) && (!hiringNowOnly || person.isHiringNow)
                && (!unansweredOnly || person.isUnanswered)
        }
    }

    /// "42 people at 15 companies".
    public static func describeCounts(_ people: [RelatedPerson]) -> String {
        let companies = Set(people.compactMap { person in person.companyID?.uuidString ?? person.companyName?.lowercased() }).count
        let peopleText = people.count == 1 ? "1 person" : "\(people.count) people"
        return companies == 0 ? peopleText : peopleText + " at " + (companies == 1 ? "1 company" : "\(companies) companies")
    }
}

/// One message of a LinkedIn conversation.
public struct LinkedInMessage: Decodable, Equatable, Sendable {
    public var senderName: String
    public var sentAt: Date
    public var subject: String?
    public var content: String
}

public struct ConversationMessagesResponse: Decodable, Sendable {
    public var messages: [LinkedInMessage]
}

private extension String {
    func capitalizingFirstLetter() -> String {
        prefix(1).uppercased() + dropFirst()
    }
}
