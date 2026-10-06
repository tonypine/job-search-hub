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
    /// When a person at the company first wrote back, as the hub read it in
    /// the mail; nil until someone does.
    public var contactedAt: Date?
    /// When the application went out, the first time it reached Applied or
    /// a later phase; kept when the card closes, nil for one never sent.
    public var appliedAt: Date?
    public var createdAt: Date
    public var updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, closedReason, notes, phaseEnteredAt, lastFollowedUpAt, contactedAt, appliedAt, createdAt, updatedAt
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
    /// When the card was dismissed as not a good fit, and why: its job's
    /// dismissal, or its own for a card with no job.
    public var dismissedAt: Date?
    public var dismissalReason: String?

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

    /// The one thing the card says about where it stands, the most urgent
    /// first: a follow-up overdue or due today, then that someone wrote
    /// back, then when the next follow-up falls due; nil for a card with
    /// none of them.
    public func getStatus(now: Date, calendar: Calendar = .current) -> PipelineCardStatus? {
        let followUp = getFollowUpStatus(now: now, calendar: calendar)
        if let followUp, followUp.isDue {
            return .followUp(followUp)
        }
        if let contactedAt = application.contactedAt {
            return .heardBack(contactedAt)
        }
        return followUp.map(PipelineCardStatus.followUp)
    }

    /// What the card keeps off its face for its tooltip: the owner's notes,
    /// why it closed and why it was skipped, one per line; empty for none.
    public var tooltip: String {
        var lines: [String] = []
        if let notes = application.notes?.trimmingCharacters(in: .whitespacesAndNewlines), !notes.isEmpty {
            lines.append(notes)
        }
        if let closedReason = application.closedReason, !closedReason.isEmpty {
            lines.append("Closed: \(closedReason)")
        }
        if let dismissalReason, !dismissalReason.isEmpty {
            lines.append("Skipped: \(dismissalReason)")
        }
        return lines.joined(separator: "\n")
    }

    /// The job's title, or the company's name for an application with no job.
    public var title: String { jobTitle ?? companyName ?? "Untitled" }

    public func getDaysInPhase(now: Date) -> Int {
        Calendar.current.dateComponents([.day], from: application.phaseEnteredAt, to: now).day ?? 0
    }

    enum CodingKeys: String, CodingKey {
        case application, jobTitle, companyName, followUpDueAt, unseenUpdates, dismissedAt, dismissalReason
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

    /// The phases a card is still moving through, in order: every phase but
    /// the closed ones.
    public var openPhases: [PipelinePhase] { phases.filter { !$0.isClosed } }

    /// The phase a card goes to when it ends; nil on a board without one.
    public var closedPhase: PipelinePhase? { phases.first(where: \.isClosed) }

    /// The cards that ended, newest first.
    public var closedCards: [PipelineCard] {
        let closedPhaseIDs = Set(phases.filter(\.isClosed).map(\.id))
        return cards.filter { closedPhaseIDs.contains($0.application.phaseID) }
    }

    /// The cards still moving through the phases.
    public var openCards: [PipelineCard] {
        let openPhaseIDs = Set(openPhases.map(\.id))
        return cards.filter { openPhaseIDs.contains($0.application.phaseID) }
    }

    /// How many of a phase's cards have a follow-up due today or overdue,
    /// for its column's header.
    public func getDueTally(in phase: PipelinePhase, now: Date, calendar: Calendar = .current) -> DueTally {
        var tally = DueTally()
        for card in getCards(in: phase) {
            switch card.getFollowUpStatus(now: now, calendar: calendar) {
            case .overdue:
                tally.due += 1
                tally.overdue += 1
            case .dueToday:
                tally.due += 1
            case .dueIn, nil:
                break
            }
        }
        return tally
    }

    /// How many applications went out and how many a person answered, so a
    /// run of unanswered ones shows before it costs weeks. An open card went
    /// out once it reached Applied, or every phase after the first when none
    /// is named so; it was answered when someone at the company wrote back or
    /// the owner moved it past Applied. A closed card counts when the hub
    /// dated it gone out, and was answered when someone wrote back, so a
    /// rejection stays in the tally and a card dropped while saved doesn't.
    public func getContactTally(now: Date, calendar: Calendar = .current) -> ContactTally {
        let reading = ContactReading(phases: phases)
        let closedPhaseIDs = Set(phases.filter(\.isClosed).map(\.id))
        var tally = ContactTally()
        for card in cards {
            if closedPhaseIDs.contains(card.application.phaseID) {
                guard card.application.appliedAt != nil else { continue }
                tally.sent += 1
                if card.application.contactedAt != nil {
                    tally.heardBack += 1
                }
                continue
            }
            guard let answer = reading.getAnswer(to: card) else { continue }
            tally.sent += 1
            if answer == .heardBack {
                tally.heardBack += 1
            } else if card.getFollowUpStatus(now: now, calendar: calendar)?.isDue == true {
                tally.unansweredPastFollowUp += 1
            }
        }
        return tally
    }

    /// The cards the tally counts as unanswered past follow-up: they went
    /// out, nobody answered, and their follow-up is due, so waiting longer
    /// rarely helps and someone else at the company is the next route.
    public func getUnansweredPastFollowUp(now: Date, calendar: Calendar = .current) -> Set<UUID> {
        let reading = ContactReading(phases: phases)
        return Set(cards.filter { card in
            reading.getAnswer(to: card) == .unanswered && card.getFollowUpStatus(now: now, calendar: calendar)?.isDue == true
        }.map(\.id))
    }

    /// How many cards' follow-ups are due today or overdue: the Pipeline's
    /// badge in the sidebar, and what its Due scope shows.
    public func getDueCount(now: Date, calendar: Calendar = .current) -> Int {
        cards.count { $0.getFollowUpStatus(now: now, calendar: calendar)?.isDue == true }
    }

    /// How many cards' follow-ups are past their day: what turns the
    /// Pipeline's badge red.
    public func getOverdueCount(now: Date, calendar: Calendar = .current) -> Int {
        cards.count { card in
            if case .overdue = card.getFollowUpStatus(now: now, calendar: calendar) { return true }
            return false
        }
    }

    /// Puts the server's answer in place of the card's application, keeping
    /// the job and company the card shows.
    public mutating func replaceApplication(_ application: Application) {
        guard let index = cards.firstIndex(where: { $0.id == application.id }) else { return }
        cards[index].application = application
        cards.sort { $0.application.phaseEnteredAt > $1.application.phaseEnteredAt }
    }
}

/// The follow-ups due in a phase: today's and the overdue ones among them.
public struct DueTally: Equatable, Sendable {
    public var due: Int
    public var overdue: Int

    public init(due: Int = 0, overdue: Int = 0) {
        self.due = due
        self.overdue = overdue
    }

    /// "2 due", or nil when nothing is.
    public var text: String? { due > 0 ? "\(due) due" : nil }
}

/// The one line a card says about where it stands.
public enum PipelineCardStatus: Equatable, Sendable {
    /// When the next follow-up falls due, or that it is due or overdue.
    case followUp(FollowUpStatus)
    /// When someone at the company first wrote back.
    case heardBack(Date)

    /// Due today or overdue: the card gets an edge in its tone.
    public var isDue: Bool {
        if case let .followUp(status) = self { return status.isDue }
        return false
    }

    /// Overdue: the card offers its next route.
    public var isOverdue: Bool {
        if case .followUp(.overdue) = self { return true }
        return false
    }
}

/// Whether an open card went out and was answered, read from the open phases.
private struct ContactReading {
    enum Answer {
        case heardBack
        case unanswered
    }

    private let applied: PipelinePhase?
    private let firstSentPosition: Int?
    private let positions: [UUID: Int]

    init(phases: [PipelinePhase]) {
        let openPhases = phases.filter { !$0.isClosed }
        applied = openPhases.first { $0.name.caseInsensitiveCompare("Applied") == .orderedSame }
        firstSentPosition = applied?.position ?? openPhases.dropFirst().first?.position
        positions = Dictionary(uniqueKeysWithValues: openPhases.map { ($0.id, $0.position) })
    }

    /// How a sent card was answered; nil for a card that hasn't gone out or
    /// is closed.
    func getAnswer(to card: PipelineCard) -> Answer? {
        guard let position = positions[card.application.phaseID] else { return nil }
        let isPastApplied = applied.map { position > $0.position } ?? false
        if card.application.contactedAt != nil || isPastApplied {
            return .heardBack
        }
        return firstSentPosition.map { position >= $0 } == true ? .unanswered : nil
    }
}

/// The applications that went out, and how they were answered.
public struct ContactTally: Equatable, Sendable {
    public var sent: Int
    /// Sent applications a person at the company answered.
    public var heardBack: Int
    /// Open sent applications nobody answered whose follow-up is due.
    public var unansweredPastFollowUp: Int

    public init(sent: Int = 0, heardBack: Int = 0, unansweredPastFollowUp: Int = 0) {
        self.sent = sent
        self.heardBack = heardBack
        self.unansweredPastFollowUp = unansweredPastFollowUp
    }

    /// "heard back on 2 of 25 sent · 18 unanswered past follow-up", or nil
    /// when nothing went out yet.
    public var text: String? {
        guard sent > 0 else { return nil }
        let answered = "heard back on \(heardBack) of \(sent) sent"
        guard unansweredPastFollowUp > 0 else { return answered }
        return "\(answered) · \(unansweredPastFollowUp) unanswered past follow-up"
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

public extension PipelinePhase {
    /// What an empty column says goes there: the default phases' own words,
    /// or the phase's name for one the owner added.
    var emptyHint: String {
        switch name.lowercased() {
        case "saved": "Drop a card here to keep a job for later"
        case "applied": "Drop a card here once the application goes out"
        case "in contact": "Drop a card here when someone writes back"
        case "interviewing": "Drop a card here when interviews start"
        case "offer": "Drop a card here when an offer comes in"
        default: "Drop a card here when it reaches \(name)"
        }
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
        case .dueToday: "Follow-up due today"
        case let .overdue(days): days == 1 ? "Follow-up overdue 1 day" : "Follow-up overdue \(days) days"
        }
    }
}

public struct FollowUpRequest: Encodable, Sendable {
    public var note: String

    public init(note: String) {
        self.note = note
    }
}

/// A cold message to someone at a company, in the owner's words; the hub
/// dates it today and has its follow-up fall due a week later.
public struct OutreachRequest: Encodable, Equatable, Sendable {
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

/// Why a card isn't a good fit, in the owner's words; it may be empty.
public struct DismissApplicationRequest: Encodable, Equatable, Sendable {
    public var note: String

    public init(note: String) {
        self.note = note
    }
}

public extension HubClient {
    /// The board: every card not dismissed.
    func getPipeline() async throws -> PipelineResponse {
        try await get("v1/pipeline", as: PipelineResponse.self)
    }

    /// The cards dismissed as not a good fit, in the phases they left.
    func getDismissedPipeline() async throws -> PipelineResponse {
        try await get("v1/pipeline", query: [URLQueryItem(name: "dismissed", value: "true")], as: PipelineResponse.self)
    }

    /// Takes the card off the board as not a good fit; a card's job leaves the Jobs list too.
    func dismissApplication(_ id: UUID, note: String) async throws -> Application {
        let request = DismissApplicationRequest(note: note.trimmingCharacters(in: .whitespacesAndNewlines))
        return try await send("POST", "v1/applications/\(id.uuidString)/dismiss", body: request, as: ApplicationResponse.self).application
    }

    /// Records a cold message to someone at the company: its outreach card
    /// moves to Applied, added when it has none, or counts a follow-up when
    /// it is already there or further on.
    func recordOutreach(companyID: UUID, note: String) async throws -> ApplicationResponse {
        let request = OutreachRequest(note: note.trimmingCharacters(in: .whitespacesAndNewlines))
        return try await send("POST", "v1/companies/\(companyID.uuidString)/outreach", body: request, as: ApplicationResponse.self)
    }

    /// Puts a dismissed card back on the board, in the phase it left.
    func restoreApplication(_ id: UUID) async throws -> Application {
        try await send("POST", "v1/applications/\(id.uuidString)/restore", body: EmptyBody(), as: ApplicationResponse.self).application
    }
}
