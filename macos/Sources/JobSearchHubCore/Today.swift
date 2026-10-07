import Foundation

/// What Today, the page that answers "what should I do now?", takes from the
/// hub's lists: the decision queue, the pipeline, the unseen updates, the
/// people, for the recruiters among them, and the features waiting on the
/// owner's first use. Android's `Today` reads the first four the same way.
public enum Today {
    /// How many jobs to decide Today shows, the best first.
    public static let decisionCount = 3

    /// Updates about someone writing back.
    static let replyKinds: Set<String> = ["human_reply", "interview_invite", "rejection", "recruiter_outreach"]

    /// The updates Today lists: replies, confirmations and fresh strong
    /// matches. Follow-ups due show as Follow up, and the hub's own work
    /// stays in the full history.
    static let newsKinds = replyKinds.union(["application_confirmation", "fresh_match"])

    /// The jobs to decide first, in the queue's order: undecided before
    /// those left for later, then by match.
    public static func getTopDecisions(_ queue: [DecisionQueueItem]) -> [DecisionQueueItem] {
        Array(queue.prefix(decisionCount))
    }

    /// The cards whose follow-up is due: the most overdue first, then those
    /// due today.
    public static func getDueFollowUps(_ board: PipelineBoard, now: Date, calendar: Calendar = .current) -> [DueFollowUp] {
        board.cards
            .compactMap { card in
                card.getFollowUpStatus(now: now, calendar: calendar).flatMap { $0.isDue ? DueFollowUp(card: card, status: $0) : nil }
            }
            .sorted { ($0.card.followUpDueAt ?? .distantPast) < ($1.card.followUpDueAt ?? .distantPast) }
    }

    /// The unseen updates Today lists, newest first as the hub sends them.
    public static func getUnseenNews(_ updates: [HubUpdate]) -> [HubUpdate] {
        updates.filter { $0.isUnseen && newsKinds.contains($0.kind) }
    }

    /// The recruiters worth a reply now: unanswered, at a company with jobs
    /// that fit, the latest message first.
    public static func getWaitingRecruiters(_ people: [RelatedPerson]) -> [RelatedPerson] {
        people.filter { $0.relation == .recruiter && $0.isUnanswered && $0.fittingJobs > 0 }
            .sorted { ($0.lastContactAt ?? .distantPast) > ($1.lastContactAt ?? .distantPast) }
    }

    /// The Get started card's rows, one for each feature waiting on the
    /// owner's first use, in the hub's order. A kind this app doesn't know
    /// is left out, and with no rows the card is left out.
    public static func getStartRows(_ steps: [FirstStep]) -> [StartRow] {
        steps.compactMap { step in
            switch step.kind {
            case FirstStep.profileInterviewKind:
                let confirmed = step.confirmedEntries ?? 0
                return StartRow(
                    id: step.kind, title: "Enhance your profile",
                    detail: "\(confirmed) of \(FirstStep.confirmedEntriesToTailor) entries confirmed. An interview turns what you remember into entries.",
                    button: "Enhance profile", symbol: "bubble.left.and.text.bubble.right", destination: .profileInterview
                )
            case FirstStep.tailoredCVKind:
                return StartRow(
                    id: step.kind, title: "Draft a tailored CV",
                    detail: "Pursue a job you like and the hub drafts a CV for it from your confirmed entries.",
                    button: "Open Decide", symbol: "doc.text", destination: .decide
                )
            case FirstStep.modelComparisonKind:
                guard let comparisonID = step.comparisonID else { return nil }
                let fields = step.unjudgedFields ?? 0
                return StartRow(
                    id: step.kind, title: "Judge “\(step.comparisonTitle ?? "the comparison")”",
                    detail: "\(fields == 1 ? "1 field has" : "\(fields) fields have") no verdict yet. Mark the models' answers right or wrong.",
                    button: "Open comparison", symbol: "square.split.2x1", destination: .comparison(comparisonID)
                )
            default:
                return nil
            }
        }
    }

    /// The chips that sum Today up: "7 to decide · 1 overdue · 1 due today ·
    /// 2 replies". A count of nothing leaves its chip out.
    public static func getChips(toDecide: Int, followUps: [DueFollowUp], updates: [HubUpdate]) -> [TodayChip] {
        let overdue = followUps.count { $0.status.isDue && $0.status != .dueToday }
        let dueToday = followUps.count { $0.status == .dueToday }
        let replies = getUnseenNews(updates).count { replyKinds.contains($0.kind) }
        let chips: [TodayChip?] = [
            toDecide > 0 ? TodayChip(text: "\(toDecide) to decide", tone: .accent, symbol: nil) : nil,
            overdue > 0 ? TodayChip(text: "\(overdue) overdue", tone: .negative, symbol: "bell.fill") : nil,
            dueToday > 0 ? TodayChip(text: "\(dueToday) due today", tone: .caution, symbol: "bell") : nil,
            replies > 0 ? TodayChip(text: replies == 1 ? "1 reply" : "\(replies) replies", tone: .positive, symbol: "arrowshape.turn.up.left.fill") : nil,
        ]
        return chips.compactMap { $0 }
    }

    /// The chips as one line, or what Today says when nothing needs you.
    public static func summarize(_ chips: [TodayChip]) -> String {
        chips.isEmpty ? "Nothing needs you now" : chips.map(\.text).joined(separator: " · ")
    }
}

/// A pipeline card due a follow-up, and how due.
public struct DueFollowUp: Equatable, Identifiable, Sendable {
    public var card: PipelineCard
    public var status: FollowUpStatus

    public var id: UUID { card.id }
}

/// A feature that gives nothing until the owner first uses it, as the hub
/// reads it from its data: it leaves once the owner has done the step.
public struct FirstStep: Decodable, Equatable, Sendable {
    public static let profileInterviewKind = "profile_interview"
    public static let tailoredCVKind = "tailored_cv"
    public static let modelComparisonKind = "model_comparison"
    /// The confirmed entries the interview's step asks for.
    public static let confirmedEntriesToTailor = 5

    public var kind: String
    public var confirmedEntries: Int?
    public var comparisonID: UUID?
    public var comparisonTitle: String?
    public var unjudgedFields: Int?

    public init(kind: String, confirmedEntries: Int? = nil, comparisonID: UUID? = nil, comparisonTitle: String? = nil, unjudgedFields: Int? = nil) {
        self.kind = kind
        self.confirmedEntries = confirmedEntries
        self.comparisonID = comparisonID
        self.comparisonTitle = comparisonTitle
        self.unjudgedFields = unjudgedFields
    }

    enum CodingKeys: String, CodingKey {
        case kind, confirmedEntries, comparisonTitle, unjudgedFields
        case comparisonID = "comparisonId"
    }
}

public struct FirstStepsResponse: Decodable, Sendable {
    public var steps: [FirstStep]
}

/// Where a Get started row's button lands.
public enum StartDestination: Equatable, Sendable {
    /// The Profile page, with the interview beside it.
    case profileInterview
    case decide
    /// Model lab, on the comparison.
    case comparison(UUID)
}

/// One row of Today's Get started card.
public struct StartRow: Equatable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var detail: String
    public var button: String
    public var symbol: String
    public var destination: StartDestination
}

/// One count at the top of Today, in its tone.
public struct TodayChip: Equatable, Identifiable, Sendable {
    public var text: String
    public var tone: Tone
    public var symbol: String?

    public var id: String { text }
}

/// What the hub's agents and local models are doing now, for Today's Hub
/// card.
public enum HubActivity {
    /// A line for each agent run going on, then one for the local model's
    /// call and how many wait behind it. Empty while nothing runs, paused
    /// work included.
    public static func describe(work: ModelWork?, agentRuns: [AgentRun]) -> [String] {
        var lines = agentRuns.filter(\.isRunning).map { run in
            run.input.isEmpty ? RunsSummary.getKindTitle(run.kind) : "\(RunsSummary.getKindTitle(run.kind)): \(run.input)"
        }
        if let work {
            if let running = work.running {
                let waiting = work.waiting.isEmpty ? "" : " · \(work.waiting.count) waiting"
                lines.append("\(RunsSummary.getKindTitle(running.kind)) on the local model\(waiting)")
            } else if work.runtime.state == "loading" {
                lines.append("Loading \(work.runtime.model ?? "a local model")")
            }
        }
        return lines
    }
}
