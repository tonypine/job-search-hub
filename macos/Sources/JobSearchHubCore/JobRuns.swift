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
    /// A start takes the owner's note on what to fix.
    public var needsNote: Bool

    public var isAgent: Bool { engine == "claude_agent" }
}

public struct RunnableKindsResponse: Decodable, Equatable, Sendable {
    public var kinds: [RunnableKind]
}

/// Starts a kind's run for a job or a company now, on its kind's model and
/// whatever its pause. A fix takes its note.
public struct StartRunRequest: Encodable, Equatable, Sendable {
    public var kind: String
    public var subjectType: String
    public var subjectID: UUID
    public var note: String?

    public init(kind: String, subjectType: String, subjectID: UUID, note: String? = nil) {
        self.kind = kind
        self.subjectType = subjectType
        self.subjectID = subjectID
        self.note = note
    }

    enum CodingKeys: String, CodingKey {
        case kind, subjectType, note
        case subjectID = "subjectId"
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

    /// The rows in the hub's order, then those that ask for a note, such as
    /// the fix of the job's details, under a separator.
    public static func split(_ kinds: [RunnableKind]) -> (runs: [RunnableKind], withNote: [RunnableKind]) {
        (kinds.filter { !$0.needsNote }, kinds.filter(\.needsNote))
    }

    /// A row's name: one run of the kind, "Full brief", where the hub names
    /// the kind by its runs, "Full briefs".
    public static func getTitle(of kind: RunnableKind) -> String {
        switch kind.id {
        case "job_facts": "Job facts"
        case "job_brief": "Brief"
        case "job_brief_full": "Full brief"
        case "job_cv": "Tailored CV"
        case "recruiter_screen": "Recruiter screen"
        case "interview_prep": "Interview pack"
        case fixKind: "Fix the job's details"
        default: kind.title
        }
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

    /// The ▶ button's tooltip: what it starts, why it can't, and that a
    /// paused kind runs anyway.
    public static func describeStart(of kind: RunnableKind) -> String {
        let title = getTitle(of: kind)
        guard kind.canRun else { return capitalizeFirst(kind.reason ?? "Can't run yet") }
        if kind.needsNote { return "Say what's wrong, and an agent corrects the job's details" }
        let start = "Start \(title.lowercasedFirst) now"
        return kind.paused ? "\(start). \(kind.title) are paused, but a run you start goes ahead anyway." : start
    }

    /// The model the kind runs on, with its provider's mark: its route's,
    /// or Claude for an agent with none; nil when nothing routes it.
    public static func getModel(of kind: RunnableKind, providers: [ModelProvider]) -> RunKindModel? {
        if let route = kind.route {
            let providerKind = providers.first { $0.id == route.providerID }?.kind ?? ""
            return RunKindModel(name: RunKinds.describeModel(route.model), providerName: route.provider, providerKind: providerKind)
        }
        if kind.isAgent {
            return RunKindModel(name: "Claude", providerName: "Claude", providerKind: RunKindModel.claudeProviderKind)
        }
        return nil
    }

    /// Whether the run started has landed: its record, or a later one of
    /// the kind for the job, has finished. `before` is the kind's last run
    /// when it started.
    public static func hasLanded(_ started: UUID, of kind: RunnableKind, before: UUID?) -> Bool {
        guard let last = kind.lastRun, last.outcome != "running" else { return false }
        return last.id == started || last.id != before
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
