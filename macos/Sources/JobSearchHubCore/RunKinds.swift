import Foundation

/// A group of run kinds, by what they work on: Jobs, Companies, Mail and
/// messages, You, Sources.
public struct RunGroup: Decodable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var title: String
}

/// A kind of run the hub records, with what it is doing now: whether it's
/// paused, how many of its runs run and how many of its model calls wait.
public struct RunKind: Decodable, Equatable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var description: String
    public var group: String
    /// What its runs are for: job, company, mail_message,
    /// linkedin_conversation, profile or none.
    public var subjectType: String
    /// model_call, claude_one_shot or claude_agent.
    public var engine: String
    /// The hub starts it by itself; otherwise it runs only when the owner
    /// asks, and has no pause.
    public var automatic: Bool
    public var paused: Bool
    public var running: Int
    public var waiting: Int

    /// Only what the hub starts by itself pauses: runs the owner starts
    /// always go ahead.
    public var isPausable: Bool { automatic }

    public var runsOnClaude: Bool { engine == "claude_one_shot" || engine == "claude_agent" }

    /// A Claude Code agent, which the hub routes to the Claude subscription
    /// only, with no fallback.
    public var isAgent: Bool { engine == "claude_agent" }

    /// What the kind is doing, the first that holds: paused, running,
    /// waiting, failed lately, then idle or on demand.
    public func getStatus(failedCount: Int) -> RunKindStatus {
        if automatic && paused { return .paused }
        if running > 0 { return .running }
        if waiting > 0 { return .waiting }
        if failedCount > 0 { return .failed }
        return automatic ? .idle : .onDemand
    }

    /// The word at the right of its row: "6 waiting", "paused · 3",
    /// "1 failed", "on demand".
    public func getStateWord(failedCount: Int) -> String {
        switch getStatus(failedCount: failedCount) {
        case .paused: waiting > 0 ? "paused · \(waiting)" : "paused"
        case .running: waiting > 0 ? "running · \(waiting) waiting" : "running"
        case .waiting: "\(waiting) waiting"
        case .failed: "\(failedCount) failed"
        case .idle: "idle"
        case .onDemand: "on demand"
        }
    }
}

/// What a kind of run, or every kind, is doing, as its row's glyph and its
/// page's chip show it.
public enum RunKindStatus: Equatable, Sendable {
    case running
    case waiting
    case idle
    case paused
    case failed
    case onDemand

    public var title: String {
        switch self {
        case .running: "Running"
        case .waiting: "Waiting"
        case .idle: "Idle"
        case .paused: "Paused"
        case .failed: "Failed"
        case .onDemand: "On demand"
        }
    }

    public var symbolName: String {
        switch self {
        case .running: "play.circle.fill"
        case .waiting: "clock"
        case .idle: "circle"
        case .paused: "pause.circle.fill"
        case .failed: "exclamationmark.circle.fill"
        case .onDemand: "hand.tap"
        }
    }

    public var tone: Tone {
        switch self {
        case .running: .accent
        case .paused: .caution
        case .failed: .negative
        case .waiting, .idle, .onDemand: .neutral
        }
    }
}

public struct RunKindsResponse: Decodable, Equatable, Sendable {
    public var groups: [RunGroup]
    public var kinds: [RunKind]
}

/// A group of the run kinds column, with its kinds in the hub's order.
public struct RunKindSection: Identifiable, Equatable, Sendable {
    public var group: RunGroup
    public var kinds: [RunKind]

    public var id: String { group.id }
}

/// Every kind at once, for the All kinds row and page: what runs, what
/// waits, and how many of the kinds the hub starts by itself are paused.
public struct AllRunKinds: Equatable, Sendable {
    public var running: Int
    public var waiting: Int
    public var failed: Int
    public var automaticCount: Int
    public var pausedCount: Int

    public init(_ kinds: [RunKind], failedCounts: [String: Int] = [:]) {
        running = kinds.reduce(0) { $0 + $1.running }
        waiting = kinds.reduce(0) { $0 + $1.waiting }
        failed = kinds.reduce(0) { $0 + failedCounts[$1.id, default: 0] }
        automaticCount = kinds.count(where: \.automatic)
        pausedCount = kinds.count { $0.automatic && $0.paused }
    }

    public var isEveryKindPaused: Bool { automaticCount > 0 && pausedCount == automaticCount }
    public var canPauseAll: Bool { pausedCount < automaticCount }
    public var canResumeAll: Bool { pausedCount > 0 }

    public var status: RunKindStatus {
        if isEveryKindPaused { return .paused }
        if running > 0 { return .running }
        if waiting > 0 { return .waiting }
        return failed > 0 ? .failed : .idle
    }

    /// The word at the right of the All kinds row.
    public var stateWord: String {
        switch status {
        case .paused: "paused"
        case .running: "\(running) running"
        case .waiting: "\(waiting) waiting"
        case .failed: "\(failed) failed"
        case .idle, .onDemand: "idle"
        }
    }

    /// What runs and waits: "1 running · 6 waiting", "Nothing running".
    public var activityLine: String {
        let parts = [running > 0 ? "\(running) running" : nil, waiting > 0 ? "\(waiting) waiting" : nil].compactMap { $0 }
        return parts.isEmpty ? "Nothing running" : parts.joined(separator: " · ")
    }

    /// "3 of 9 automatic kinds paused"; nil while none is.
    public var pausedLine: String? {
        guard pausedCount > 0 else { return nil }
        return "\(pausedCount) of \(automaticCount) automatic \(automaticCount == 1 ? "kind" : "kinds") paused"
    }
}

/// The hub's work in three lines, for the All kinds page: what runs now,
/// what runs next, and what waits behind a pause.
public struct RunQueueStrip: Equatable, Sendable {
    public var now: String
    public var next: String
    public var held: String
}

/// The model a kind runs on, and its provider, whose mark shows beside it.
public struct RunKindModel: Equatable, Sendable {
    public static let claudeProviderKind = ModelProvider.claudeSubscriptionKind

    public var name: String
    public var providerName: String
    public var providerKind: String

    /// The provider's mark: a chip for the hub's runtime, a rack for a
    /// model server, a branch for OpenRouter, a sparkle for Claude.
    public var markSymbol: String {
        RunKinds.getProviderMark(providerKind)
    }

    /// What its row and header say of it: "Runs on Claude Sonnet from
    /// Claude", without the provider until its name is read.
    public var runsOnText: String {
        providerName.isEmpty ? "Runs on \(name)" : "Runs on \(name) from \(providerName)"
    }
}

public enum RunKinds {
    /// The kinds by group, in the hub's group order, then any group the hub
    /// didn't name, last. Groups without kinds are left out.
    public static func group(_ kinds: [RunKind], in groups: [RunGroup]) -> [RunKindSection] {
        var sections = groups.map { group in RunKindSection(group: group, kinds: kinds.filter { $0.group == group.id }) }
        let known = Set(groups.map(\.id))
        for kind in kinds where !known.contains(kind.group) {
            if let index = sections.firstIndex(where: { $0.group.id == kind.group }) {
                sections[index].kinds.append(kind)
            } else {
                let title = kind.group.replacingOccurrences(of: "_", with: " ").capitalized
                sections.append(RunKindSection(group: RunGroup(id: kind.group, title: title), kinds: [kind]))
            }
        }
        return sections.filter { !$0.kinds.isEmpty }
    }

    /// The model kind runs on: its route's, by the name its provider's
    /// catalog gives it when that is read, or Claude for a kind that runs on
    /// Claude without one; nil when nothing routes it. A route on a provider
    /// the list doesn't have yet, one set up since it was read, is named
    /// all the same, without its provider.
    public static func getModel(
        for kind: RunKind, routes: [TaskRoute], providers: [ModelProvider], catalogs: [UUID: [ProviderModel]] = [:]
    ) -> RunKindModel? {
        if let route = routes.first(where: { $0.kind == kind.id }) {
            let name = ModelChoices.getModelName(RoutedModel(providerID: route.providerID, model: route.model), catalogs: catalogs)
            let provider = providers.first { $0.id == route.providerID }
            return RunKindModel(name: name, providerName: provider?.name ?? "", providerKind: provider?.kind ?? "")
        }
        if kind.runsOnClaude {
            return RunKindModel(name: "Claude", providerName: "Claude", providerKind: RunKindModel.claudeProviderKind)
        }
        return nil
    }

    public static func getProviderMark(_ providerKind: String) -> String {
        switch providerKind {
        case ModelProvider.hubRuntimeKind: "cpu"
        case ModelProvider.openAICompatibleKind: "server.rack"
        case ModelProvider.openRouterKind: "arrow.triangle.branch"
        case RunKindModel.claudeProviderKind: "sparkle"
        default: "cube"
        }
    }

    /// A model's name as the owner reads it: a local file without its
    /// ".gguf".
    public static func describeModel(_ model: String) -> String {
        model.hasSuffix(".gguf") ? String(model.dropLast(5)) : model
    }

    /// Each kind's failed runs in a window of run metrics, by kind.
    public static func countFailures(_ metrics: RunMetrics) -> [String: Int] {
        var counts: [String: Int] = [:]
        for bucket in metrics.buckets {
            for kind in bucket.kinds where kind.failed > 0 {
                counts[kind.kind, default: 0] += kind.failed
            }
        }
        return counts
    }

    /// What runs now, what runs next, and what waits behind a pause: the
    /// kinds with a run going, the first call in the queue that isn't held,
    /// and the paused kinds' waiting calls.
    public static func describeQueue(_ kinds: [RunKind], work: ModelWork?) -> RunQueueStrip {
        let titles = Dictionary(kinds.map { ($0.id, $0.title) }, uniquingKeysWith: { first, _ in first })
        let title = { (kind: String) in titles[kind] ?? RunsSummary.getKindTitle(kind) }
        let paused = Set(kinds.filter { $0.automatic && $0.paused }.map(\.id))
        let running = kinds.filter { $0.running > 0 }.map { $0.running > 1 ? "\($0.title) (\($0.running))" : $0.title }
        let next = work?.waiting.first { $0.priority == "dispatched" || !paused.contains($0.kind) }
        let held = kinds.filter { paused.contains($0.id) && $0.waiting > 0 }.map { "\($0.title) \($0.waiting)" }
        return RunQueueStrip(
            now: running.isEmpty ? "Nothing" : running.joined(separator: ", "),
            next: next.map { "\(title($0.kind)) on \(describeModel($0.model))" } ?? "Nothing waits",
            held: held.isEmpty ? "Nothing" : held.joined(separator: ", ")
        )
    }
}

/// The counts of a window of runs, as `GET /v1/runs/metrics` gives them:
/// the window's totals, the totals of the window of the same length just
/// before it, and each bucket of the window, the empty ones included.
public struct RunMetrics: Decodable, Equatable, Sendable {
    /// The runs of a window, from both of the hub's records.
    public struct Totals: Decodable, Equatable, Sendable {
        public var from: Date
        public var to: Date
        public var runs: Int
        public var failed: Int
        public var invalid: Int
        public var inputTokens: Int
        public var outputTokens: Int
        /// Over the runs that finished; nil when none did.
        public var averageDurationMS: Int?
        /// What the runs that report a cost cost, as text so the decimal is
        /// never rounded.
        public var costUSD: String
        /// The model calls on the owner's Claude plan, which cost nothing
        /// more and count as nothing in the cost.
        public var claudePlanRuns: Int

        enum CodingKeys: String, CodingKey {
            case from, to, runs, failed, invalid, inputTokens, outputTokens, claudePlanRuns
            case averageDurationMS = "averageDurationMs"
            case costUSD = "costUsd"
        }

        public init(
            from: Date, to: Date, runs: Int = 0, failed: Int = 0, invalid: Int = 0, inputTokens: Int = 0, outputTokens: Int = 0,
            averageDurationMS: Int? = nil, costUSD: String = "0", claudePlanRuns: Int = 0
        ) {
            self.from = from
            self.to = to
            self.runs = runs
            self.failed = failed
            self.invalid = invalid
            self.inputTokens = inputTokens
            self.outputTokens = outputTokens
            self.averageDurationMS = averageDurationMS
            self.costUSD = costUSD
            self.claudePlanRuns = claudePlanRuns
        }

        public var tokens: Int { inputTokens + outputTokens }
    }

    /// The runs started from its start until before its end, in all and by
    /// kind. The first and last buckets are clipped to the window.
    public struct Bucket: Decodable, Equatable, Identifiable, Sendable {
        public var start: Date
        public var end: Date
        public var runs: Int
        public var failed: Int
        public var inputTokens: Int
        public var outputTokens: Int
        /// Only the kinds that ran.
        public var kinds: [KindCount]

        public init(start: Date, end: Date, runs: Int = 0, failed: Int = 0, inputTokens: Int = 0, outputTokens: Int = 0, kinds: [KindCount] = []) {
            self.start = start
            self.end = end
            self.runs = runs
            self.failed = failed
            self.inputTokens = inputTokens
            self.outputTokens = outputTokens
            self.kinds = kinds
        }

        public var id: Date { start }
        public var tokens: Int { inputTokens + outputTokens }
        public var interval: DateInterval { DateInterval(start: start, end: max(start, end)) }

        public func contains(_ date: Date) -> Bool { start <= date && date < end }
    }

    public struct KindCount: Decodable, Equatable, Sendable {
        public var kind: String
        public var runs: Int
        public var failed: Int
        public var inputTokens: Int
        public var outputTokens: Int

        public init(kind: String, runs: Int, failed: Int = 0, inputTokens: Int = 0, outputTokens: Int = 0) {
            self.kind = kind
            self.runs = runs
            self.failed = failed
            self.inputTokens = inputTokens
            self.outputTokens = outputTokens
        }

        public var tokens: Int { inputTokens + outputTokens }
    }

    /// The kind counted; empty for every kind.
    public var kind: String
    /// hour, day or week.
    public var bucket: String
    public var timeZone: String
    public var totals: Totals
    public var previous: Totals
    public var buckets: [Bucket]

    public init(kind: String = "", bucket: String, timeZone: String = "UTC", totals: Totals, previous: Totals, buckets: [Bucket]) {
        self.kind = kind
        self.bucket = bucket
        self.timeZone = timeZone
        self.totals = totals
        self.previous = previous
        self.buckets = buckets
    }

    /// The bucket a moment falls in; nil outside the window.
    public func getBucket(at date: Date) -> Bucket? {
        buckets.first { $0.contains(date) }
    }
}

/// What a run was for: a job with its company, a company, a message or a
/// conversation, or the profile. Its title is empty once the subject is
/// gone.
public struct RunSubject: Decodable, Equatable, Sendable {
    public var type: String
    public var id: UUID?
    public var title: String
    public var companyID: UUID?
    public var companyName: String?

    enum CodingKeys: String, CodingKey {
        case type, id, title, companyName
        case companyID = "companyId"
    }
}

/// A piece of a run's subject: its words, and what clicking them opens.
public struct RunSubjectPart: Equatable, Sendable {
    public var text: String
    public var opens: InspectorSubject?
}

/// One run from either of the hub's records, a model call or a Claude
/// agent run, as `GET /v1/runs` lists it.
public struct ActivityRun: Decodable, Equatable, Identifiable, Sendable {
    public static let agentRunRecord = "agent_run"

    public var id: UUID
    public var record: String
    public var kind: String
    public var subject: RunSubject?
    /// Empty for an agent run, whose model Claude Code picks.
    public var model: String
    public var provider: String
    public var startedAt: Date
    /// Nil while an agent run runs.
    public var durationMS: Int?
    /// succeeded, failed, invalid or running.
    public var outcome: String
    public var promptTokens: Int
    public var completionTokens: Int
    public var costUSDEstimate: String?
    public var error: String?
    public var startedByOwner: Bool

    enum CodingKeys: String, CodingKey {
        case id, record, kind, subject, model, provider, startedAt, outcome, promptTokens, completionTokens, error, startedByOwner
        case durationMS = "durationMs"
        case costUSDEstimate = "costUsdEstimate"
    }

    public var isAgentRun: Bool { record == Self.agentRunRecord }
    public var isFailed: Bool { outcome == "failed" || outcome == "invalid" }

    public var outcomeSymbol: String {
        switch outcome {
        case "succeeded": "checkmark.circle.fill"
        case "running": "clock"
        case "invalid": "questionmark.circle.fill"
        default: "xmark.circle.fill"
        }
    }

    /// The model that answered; an agent run's provider, Claude.
    public var modelText: String {
        model.isEmpty ? provider : RunKinds.describeModel(model)
    }

    /// What the run was for, in the pieces its row shows: a job's title then
    /// its company's name, each opening it; a company's name; a message's or
    /// conversation's subject; "Your profile". Empty when the kind is for
    /// nothing in particular.
    public var subjectParts: [RunSubjectPart] {
        guard let subject else { return [] }
        switch subject.type {
        case "job":
            var parts = [RunSubjectPart(
                text: subject.title.isEmpty ? "A deleted job" : subject.title, opens: subject.title.isEmpty ? nil : subject.id.map { .job($0) }
            )]
            if let companyName = subject.companyName, !companyName.isEmpty {
                parts.append(RunSubjectPart(text: companyName, opens: subject.companyID.map { .company($0) }))
            }
            return parts
        case "company":
            return [RunSubjectPart(
                text: subject.title.isEmpty ? "A deleted company" : subject.title, opens: subject.title.isEmpty ? nil : subject.id.map { .company($0) }
            )]
        case "mail_message":
            return [RunSubjectPart(text: subject.title.isEmpty ? "A deleted message" : subject.title)]
        case "linkedin_conversation":
            return [RunSubjectPart(text: subject.title.isEmpty ? "A deleted conversation" : subject.title)]
        case "profile":
            return [RunSubjectPart(text: "Your profile")]
        default:
            return subject.title.isEmpty ? [] : [RunSubjectPart(text: subject.title)]
        }
    }

    /// The subject as one line: "Staff engineer · Initech".
    public var subjectText: String? {
        let parts = subjectParts
        return parts.isEmpty ? nil : parts.map(\.text).joined(separator: " · ")
    }

    /// The job and the company the run was for, which its detail opens.
    public var jobID: UUID? {
        subject?.type == "job" && subject?.title.isEmpty == false ? subject?.id : nil
    }

    public var companyID: UUID? {
        switch subject?.type {
        case "job": subject?.companyID
        case "company": subject?.title.isEmpty == false ? subject?.id : nil
        default: nil
        }
    }

    /// How long it took: "4.2 s", "2 min 5 s", or "running".
    public var durationText: String {
        guard let durationMS else { return outcome == "running" ? "running" : "—" }
        return RunFormat.describeDuration(milliseconds: durationMS)
    }

    /// What it cost: an agent run's dollars, a model call's tokens in and
    /// out; nil when it reported neither.
    public var usageText: String? {
        if let costUSDEstimate, let cost = Double(costUSDEstimate) {
            return cost.formatted(.currency(code: "USD").precision(.fractionLength(2)))
        }
        guard promptTokens + completionTokens > 0 else { return nil }
        return "\(promptTokens.formatted()) → \(completionTokens.formatted()) tokens"
    }

    /// The usage short enough for a table cell, under a heading that says
    /// what it counts: the tokens without their unit, or the cost.
    public var usageCellText: String? {
        guard costUSDEstimate.flatMap(Double.init) == nil, promptTokens + completionTokens > 0 else { return usageText }
        return "\(promptTokens.formatted()) → \(completionTokens.formatted())"
    }
}

/// A page of runs, the latest first, and the cursor of the next; empty after
/// the last.
public struct ActivityRunsPage: Decodable, Equatable, Sendable {
    public var runs: [ActivityRun]
    public var nextCursor: String

    public var hasMore: Bool { !nextCursor.isEmpty }
}

/// A Claude agent run in full: what it was asked and what it answered.
public struct AgentRunDetail: Decodable, Equatable, Sendable {
    public var id: UUID
    public var kind: String
    public var input: String
    public var status: String
    public var result: JSONValue?
    public var error: String?

    /// The answer as text: indented JSON, or nothing when there is none.
    public var resultText: String? {
        guard let result, result != .null else { return nil }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        return (try? encoder.encode(result)).flatMap { String(data: $0, encoding: .utf8) }
    }
}

/// `GET /v1/agent-runs/{id}`'s answer.
public struct AgentRunDetailResponse: Decodable, Equatable, Sendable {
    public var agentRun: AgentRunDetail
}

public enum RunFormat {
    /// A run's time: "4.2 s" under a minute, then "2 min 5 s".
    public static func describeDuration(milliseconds: Int) -> String {
        let seconds = Double(milliseconds) / 1000
        if seconds < 60 { return String(format: "%.1f s", seconds) }
        let whole = Int(seconds)
        return "\(whole / 60) min \(whole % 60) s"
    }
}
