import Foundation

/// A LinkedIn conversation a recruiter started with the owner, and what their
/// company has open now.
public struct RecruiterConversation: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var title: String?
    public var startedByName: String
    public var startedByURL: String
    public var ownerWrote: Bool
    public var messageCount: Int
    public var firstMessageAt: Date?
    public var lastMessageAt: Date?
    public var classificationReason: String?
    public var hiringCompany: String?
    public var role: String?
    public var isAgency: Bool
    public var starterPosition: String?
    public var starterCompany: String?
    public var companyID: UUID?
    public var openJobs: Int
    public var fittingJobs: Int

    /// Their company has open jobs in the feed now.
    public var isHiringNow: Bool { openJobs > 0 }

    /// "3 open, 1 fits", or empty.
    public var openingsText: String {
        guard openJobs > 0 else { return "" }
        return fittingJobs > 0 ? "\(openJobs) open, \(fittingJobs) fit" : "\(openJobs) open"
    }

    enum CodingKeys: String, CodingKey {
        case id, title, startedByName, ownerWrote, messageCount, firstMessageAt, lastMessageAt, classificationReason
        case hiringCompany, role, isAgency, starterPosition, starterCompany, openJobs, fittingJobs
        case startedByURL = "startedByUrl"
        case companyID = "companyId"
    }
}

public struct RecruitersResponse: Decodable, Sendable {
    public var recruiters: [RecruiterConversation]
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

/// Which recruiters the list shows.
public struct RecruiterFilter: Equatable, Sendable {
    public var hiringNowOnly = false
    public var unansweredOnly = false

    public init(hiringNowOnly: Bool = false, unansweredOnly: Bool = false) {
        self.hiringNowOnly = hiringNowOnly
        self.unansweredOnly = unansweredOnly
    }

    public func apply(to recruiters: [RecruiterConversation]) -> [RecruiterConversation] {
        recruiters.filter { (!hiringNowOnly || $0.isHiringNow) && (!unansweredOnly || !$0.ownerWrote) }
    }
}
