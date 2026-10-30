import Foundation

/// What a kind runs on, as the hub routes it.
public struct RunnableKindRoute: Decodable, Equatable, Sendable {
    public var providerID: UUID
    public var provider: String
    public var model: String

    enum CodingKeys: String, CodingKey {
        case provider, model
        case providerID = "providerId"
    }
}

/// A kind the owner can start for a job or a company, as
/// `GET /v1/jobs/{id}/runnable-kinds` lists it: its model, its latest run
/// for the subject, and whether it can run now.
public struct RunnableKind: Decodable, Equatable, Identifiable, Sendable {
    public var id: String
    public var title: String
    public var description: String
    public var subjectType: String
    /// model_call, claude_one_shot or claude_agent.
    public var engine: String
    public var automatic: Bool
    /// The kind's own runs wait; one the owner starts doesn't.
    public var paused: Bool
    /// Nil when nothing routes it.
    public var route: RunnableKindRoute?
    /// Nil before its first run for the subject.
    public var lastRun: ActivityRun?
    public var canRun: Bool
    /// Why it can't run yet, when it can't: "needs the tailored CV first".
    public var reason: String?
    /// It can't run only for want of a model: nothing routes it, but a run
    /// on a model picked for it goes ahead.
    public var needsModel: Bool
    /// A start takes the owner's note on what to fix.
    public var needsNote: Bool
    /// The runs of the kind the owner started for the subject that the hub
    /// is still running, retries and fallbacks included. An agent's, which
    /// the Mac runs, aren't here.
    public var runningStarts: [UUID]

    public var isAgent: Bool { engine == "claude_agent" }
}

public struct RunnableKindsResponse: Decodable, Equatable, Sendable {
    public var kinds: [RunnableKind]
}

/// Starts a kind's run for a job or a company now, whatever its pause: on
/// the model chosen for this run only, else on its kind's. A fix takes its
/// note.
public struct StartRunRequest: Encodable, Equatable, Sendable {
    public var kind: String
    public var subjectType: String
    public var subjectID: UUID
    /// The provider and model chosen for this run; nil runs it on the
    /// kind's model.
    public var providerID: UUID?
    public var model: String?
    public var note: String?

    public init(kind: String, subjectType: String, subjectID: UUID, model chosen: RoutedModel? = nil, note: String? = nil) {
        self.kind = kind
        self.subjectType = subjectType
        self.subjectID = subjectID
        providerID = chosen?.providerID
        model = chosen?.model
        self.note = note
    }

    enum CodingKeys: String, CodingKey {
        case kind, subjectType, model, note
        case subjectID = "subjectId"
        case providerID = "providerId"
    }
}

/// The model select of one run control: the kind's own model, which it is
/// set to each time it shows, and the one picked for the run. Only a model
/// picked that isn't the kind's goes with the start, so the kind's own
/// model never changes.
public struct RunModelSelection: Equatable, Sendable {
    /// The kind's route; nil when nothing routes it, as for an agent that
    /// leaves the model to Claude Code.
    public var kindRoute: RunnableKindRoute?
    /// An agent runs on Claude alone, so its select lists Claude's models.
    public var isAgent: Bool
    /// The model picked for the run, which its start asks for; nil runs it
    /// on the kind's.
    public private(set) var chosen: RoutedModel?

    public init(kind: RunnableKind) {
        kindRoute = kind.route
        isAgent = kind.isAgent
    }

    /// The kind's own model, as its route names it.
    public var kindModel: RoutedModel? {
        kindRoute.map { RoutedModel(providerID: $0.providerID, model: $0.model) }
    }

    /// The model the select ticks: the one picked, else the kind's.
    public var selected: RoutedModel? { chosen ?? kindModel }

    /// Picks model for the run; the kind's own model clears the pick.
    public mutating func pick(_ model: RoutedModel) {
        chosen = model == kindModel ? nil : model
    }

    /// What the select says: the model selected by the name its catalog
    /// gives it, with its provider's mark; Claude for an agent with no
    /// route; nil when nothing routes the kind.
    public func getLabel(providers: [ModelProvider], catalogs: [UUID: [ProviderModel]]) -> RunKindModel? {
        guard let selected else {
            return isAgent ? RunKindModel(name: "Claude", providerName: "Claude", providerKind: RunKindModel.claudeProviderKind) : nil
        }
        let provider = providers.first { $0.id == selected.providerID }
        let providerName = provider?.name ?? (selected == kindModel ? kindRoute?.provider ?? "" : "")
        return RunKindModel(name: ModelChoices.getModelName(selected, catalogs: catalogs), providerName: providerName, providerKind: provider?.kind ?? "")
    }

    /// The select's models, by place, with no prices or times: Claude's
    /// alone for an agent, and the model selected kept so it shows ticked.
    public func getGroups(providers: [ModelProvider], catalogs: [UUID: [ProviderModel]]) -> [ModelChoiceGroup] {
        ModelChoices.makeCompact(ModelChoices.group(providers: providers, catalogs: catalogs, claudeOnly: isAgent, keeping: selected))
    }
}

/// A run the owner started, as its start answers: the id it is recorded
/// under once it runs, and for an agent the Mac's task it waits in.
public struct StartedRun: Decodable, Equatable, Sendable {
    public var id: UUID
    public var kind: String
    public var subjectID: UUID
    public var startedByOwner: Bool
    /// The task the Mac runs an agent kind through; nil for a model run,
    /// which the hub runs itself.
    public var taskID: UUID?

    enum CodingKeys: String, CodingKey {
        case id, kind, startedByOwner
        case subjectID = "subjectId"
        case taskID = "taskId"
    }
}

public struct StartedRunResponse: Decodable, Equatable, Sendable {
    public var queued: Bool
    public var run: StartedRun
}

/// A job's Run popover: its rows, what each says and when a run it started
/// has landed.
public enum JobRuns {
    public static let jobSubject = "job"
    public static let fixKind = "job_fix"
    /// The kinds the job's sections start: Read facts, Write full brief and
    /// Draft CV.
    public static let factsKind = "job_facts"
    public static let fullBriefKind = "job_brief_full"
    public static let cvKind = "job_cv"

    /// The rows in the hub's order, then those that ask for a note, such as
    /// the fix of the job's details, under a separator.
    public static func split(_ kinds: [RunnableKind]) -> (runs: [RunnableKind], withNote: [RunnableKind]) {
        (kinds.filter { !$0.needsNote }, kinds.filter(\.needsNote))
    }

    /// A row's name: one run of the kind, "Full brief", where the hub names
    /// the kind by its runs, "Full briefs".
    public static func getTitle(of kind: RunnableKind) -> String {
        kind.id == fixKind ? "Fix the job's details" : SubjectRuns.getRunTitle(kind.id) ?? kind.title
    }

    /// What a finished run of the kind did: "Read", "Written".
    static func getDoneVerb(of kind: String) -> String {
        switch kind {
        case "job_facts": "Read"
        case "job_brief", "job_brief_full": "Written"
        case "job_cv": "Drafted"
        case "recruiter_screen": "Screened"
        case "interview_prep": "Prepared"
        case fixKind: "Fixed"
        default: "Ran"
        }
    }

    /// The kind's last run for the job: "Read 13:12 · invalid answer",
    /// "Written 14:14", "Written Oct 27", "Never".
    public static func describeLastRun(
        _ run: ActivityRun?, now: Date, calendar: Calendar = .current, locale: Locale = .current
    ) -> String {
        guard let run else { return "Never" }
        let when: String
        if calendar.isDate(run.startedAt, inSameDayAs: now) {
            let parts = calendar.dateComponents([.hour, .minute], from: run.startedAt)
            when = String(format: "%02d:%02d", parts.hour ?? 0, parts.minute ?? 0)
        } else {
            when = run.startedAt.formatted(Date.FormatStyle(locale: locale, calendar: calendar, timeZone: calendar.timeZone).month(.abbreviated).day())
        }
        let verb = run.outcome == "running" ? "Started" : getDoneVerb(of: run.kind)
        switch run.outcome {
        case "invalid": return "\(verb) \(when) · invalid answer"
        case "failed": return "\(verb) \(when) · failed"
        case "running": return "\(verb) \(when) · running"
        default: return "\(verb) \(when)"
        }
    }

    /// A row's second line: the hub's reason when it can't run yet, else its
    /// last run, with "kind paused" while its own runs wait.
    public static func describeState(
        of kind: RunnableKind, now: Date, calendar: Calendar = .current, locale: Locale = .current
    ) -> String {
        let line = kind.canRun
            ? describeLastRun(kind.lastRun, now: now, calendar: calendar, locale: locale)
            : capitalizeFirst(kind.reason ?? "Can't run yet")
        return kind.paused ? "\(line) · kind paused" : line
    }

    /// A run's time so far: "Running 0:42", "Running 12:05",
    /// "Running 1:02:03".
    public static func describeRunning(since start: Date, now: Date) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(start)))
        let (hours, minutes, rest) = (seconds / 3600, seconds / 60 % 60, seconds % 60)
        if hours > 0 {
            return String(format: "Running %d:%02d:%02d", hours, minutes, rest)
        }
        return String(format: "Running %d:%02d", minutes, rest)
    }

    /// Whether ▶ starts the kind on the model chosen, nil for its own: when
    /// the hub can run it, or when it lacks only a model and one is picked.
    public static func canStart(_ kind: RunnableKind, on chosen: RoutedModel?) -> Bool {
        kind.canRun || (kind.needsModel && chosen != nil)
    }

    /// The ▶ button's tooltip: what it starts and on which model, why it
    /// can't, and that a paused kind runs anyway.
    public static func describeStart(of kind: RunnableKind, on chosen: RoutedModel? = nil) -> String {
        let title = getTitle(of: kind)
        guard canStart(kind, on: chosen) else {
            let reason = capitalizeFirst(kind.reason ?? "Can't run yet")
            return kind.needsModel ? "\(reason). Pick a model for this run to start it on that one" : reason
        }
        if kind.needsNote { return "Say what's wrong, and an agent corrects the job's details" }
        let start = chosen == nil ? "Start \(title.lowercasedFirst) now" : "Start \(title.lowercasedFirst) now on the model picked"
        return kind.paused ? "\(start). \(kind.title) are paused, but a run you start goes ahead anyway." : start
    }

    /// Whether the run started has landed. One the hub runs has once the
    /// kind no longer lists it running: its first record may say "invalid"
    /// while the hub asks again or falls back to another model, and the job
    /// changes after its last. An agent's, which the Mac's task runs, has
    /// once its record, under the started id, finished.
    public static func hasLanded(_ started: StartedRun, of kind: RunnableKind) -> Bool {
        guard started.taskID != nil else { return !kind.runningStarts.contains(started.id) }
        guard let last = kind.lastRun else { return false }
        return last.id == started.id && last.outcome != "running"
    }

    /// How long a started run is followed before the row stops saying it
    /// runs: the hub gives a model run ten minutes, an agent takes longer.
    public static func getFollowLimit(of kind: RunnableKind) -> TimeInterval {
        kind.isAgent ? 30 * 60 : 12 * 60
    }

    private static func capitalizeFirst(_ text: String) -> String {
        text.prefix(1).uppercased() + text.dropFirst()
    }
}

private extension String {
    var lowercasedFirst: String {
        prefix(1).lowercased() + dropFirst()
    }
}

public extension HubClient {
    /// The kinds the owner can start for the job, in the hub's order.
    func listRunnableKinds(forJob id: UUID) async throws -> [RunnableKind] {
        try await get("v1/jobs/\(id.uuidString)/runnable-kinds", as: RunnableKindsResponse.self).kinds
    }

    /// Starts the run now, ahead of background work and whatever its kind's
    /// pause.
    func startRun(_ request: StartRunRequest) async throws -> StartedRun {
        try await send("POST", "v1/runs", body: request, as: StartedRunResponse.self).run
    }
}
