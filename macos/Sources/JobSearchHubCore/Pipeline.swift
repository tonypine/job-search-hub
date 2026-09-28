import Foundation

public struct PipelinePhase: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var name: String
    public var position: Int
    /// Marks the phase where finished applications rest, with the reason they ended.
    public var isClosed: Bool
    /// Days a card may sit here, since it entered or was last followed up,
    /// before a follow-up is due; nil never falls due.
    public var followUpDays: Int?
}

public struct Application: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var jobID: UUID?
    public var companyID: UUID?
    public var phaseID: UUID
    public var closedReason: String?
    public var notes: String?
    public var phaseEnteredAt: Date
    public var lastFollowedUpAt: Date?
    public var createdAt: Date
    public var updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, closedReason, notes, phaseEnteredAt, lastFollowedUpAt, createdAt, updatedAt
        case jobID = "jobId"
        case companyID = "companyId"
        case phaseID = "phaseId"
    }
}

/// One card of the board: an application with the job and company it is for.
public struct PipelineCard: Codable, Equatable, Identifiable, Sendable {
    public var application: Application
    public var jobTitle: String?
    public var jobURL: String?
    public var companyName: String?
    /// When the card's phase wants a follow-up; nil when it asks for none.
    public var followUpDueAt: Date?
    /// Unseen updates about the card's job, or about its company when the
    /// application has no job.
    public var unseenUpdates: Int

    public var id: UUID { application.id }

    /// Where the card stands on its follow-up, by calendar day.
    public func getFollowUpStatus(now: Date, calendar: Calendar = .current) -> FollowUpStatus? {
        guard let followUpDueAt else { return nil }
        let days = calendar.dateComponents([.day], from: calendar.startOfDay(for: now), to: calendar.startOfDay(for: followUpDueAt)).day ?? 0
        switch days {
        case 0: return .dueToday
        case ..<0: return .overdue(days: -days)
        default: return .dueIn(days: days)
        }
    }

    /// The job's title, or the company's name for an application with no job.
    public var title: String { jobTitle ?? companyName ?? "Untitled" }

    public func getDaysInPhase(now: Date) -> Int {
        Calendar.current.dateComponents([.day], from: application.phaseEnteredAt, to: now).day ?? 0
    }

    enum CodingKeys: String, CodingKey {
        case application, jobTitle, companyName, followUpDueAt, unseenUpdates
        case jobURL = "jobUrl"
    }
}

public struct PipelineResponse: Codable, Equatable, Sendable {
    public var phases: [PipelinePhase]
    public var cards: [PipelineCard]
}

/// The board as the app shows it: phases in order, and each phase's cards
/// newest in the phase first, as the server lists them.
public struct PipelineBoard: Equatable, Sendable {
    public private(set) var phases: [PipelinePhase]
    public private(set) var cards: [PipelineCard]

    public init(phases: [PipelinePhase] = [], cards: [PipelineCard] = []) {
        self.phases = phases.sorted { $0.position < $1.position }
        self.cards = cards.sorted { $0.application.phaseEnteredAt > $1.application.phaseEnteredAt }
    }

    public init(_ response: PipelineResponse) {
        self.init(phases: response.phases, cards: response.cards)
    }

    public func getCards(in phase: PipelinePhase) -> [PipelineCard] {
        cards.filter { $0.application.phaseID == phase.id }
    }

    /// Puts the server's answer in place of the card's application, keeping
    /// the job and company the card shows.
    public mutating func replaceApplication(_ application: Application) {
        guard let index = cards.firstIndex(where: { $0.id == application.id }) else { return }
        cards[index].application = application
        cards.sort { $0.application.phaseEnteredAt > $1.application.phaseEnteredAt }
    }
}

public struct AddApplicationRequest: Encodable, Equatable, Sendable {
    public var jobID: UUID

    public init(jobID: UUID) {
        self.jobID = jobID
    }

    enum CodingKeys: String, CodingKey {
        case jobID = "jobId"
    }
}

public struct MoveApplicationRequest: Encodable, Equatable, Sendable {
    public var phaseID: UUID
    public var closedReason: String?

    public init(phaseID: UUID, closedReason: String? = nil) {
        self.phaseID = phaseID
        self.closedReason = closedReason
    }

    enum CodingKeys: String, CodingKey {
        case closedReason
        case phaseID = "phaseId"
    }
}

public struct ApplicationResponse: Decodable, Equatable, Sendable {
    public var application: Application
    /// False when the job was already on the pipeline.
    public var created: Bool

    enum CodingKeys: String, CodingKey {
        case application, created
    }

    public init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        application = try container.decode(Application.self, forKey: .application)
        created = try container.decodeIfPresent(Bool.self, forKey: .created) ?? false
    }
}

/// The body that adds a phase or renames one.
public struct PipelinePhaseNameRequest: Encodable, Equatable, Sendable {
    public var name: String

    public init(name: String) {
        self.name = name
    }
}

public struct ReorderPipelinePhasesRequest: Encodable, Equatable, Sendable {
    public var phaseIDs: [UUID]

    public init(phaseIDs: [UUID]) {
        self.phaseIDs = phaseIDs
    }

    enum CodingKeys: String, CodingKey {
        case phaseIDs = "phaseIds"
    }
}

public struct PipelinePhasesResponse: Decodable, Equatable, Sendable {
    public var phases: [PipelinePhase]
}

public enum PipelinePhaseOrder {
    /// The phases' IDs in order with one phase moved `offset` places, or nil
    /// when the move would take it past either end.
    public static func getIDs(of phases: [PipelinePhase], moving phaseID: UUID, by offset: Int) -> [UUID]? {
        var ids = phases.map(\.id)
        guard let index = ids.firstIndex(of: phaseID), ids.indices.contains(index + offset) else { return nil }
        ids.swapAt(index, index + offset)
        return ids
    }
}

public enum FollowUpStatus: Equatable, Sendable {
    case dueIn(days: Int)
    case dueToday
    case overdue(days: Int)

    /// Due today or earlier.
    public var isDue: Bool {
        if case .dueIn = self { return false }
        return true
    }

    public var text: String {
        switch self {
        case let .dueIn(days): days == 1 ? "Follow up tomorrow" : "Follow up in \(days) days"
        case .dueToday: "Follow up today"
        case let .overdue(days): days == 1 ? "Follow-up 1 day overdue" : "Follow-up \(days) days overdue"
        }
    }
}

public struct FollowUpRequest: Encodable, Sendable {
    public var note: String

    public init(note: String) {
        self.note = note
    }
}

/// Sets a phase's follow-up interval; no days stops it asking for follow-ups.
public struct FollowUpDaysRequest: Encodable, Sendable {
    public var days: Int?

    public init(days: Int?) {
        self.days = days
    }
}
