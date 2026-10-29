import Foundation

/// A job's or a company's Runs tab: every run for it, the latest first, a
/// page at a time.
public enum SubjectRuns {
    public static let jobSubject = "job"
    public static let companySubject = "company"
    public static let pageSize = 50
    /// The most runs the hub gives in one page.
    public static let largestPage = 200

    /// `GET /v1/runs`'s query for the subject's runs: a page of `limit`,
    /// after `cursor` when one is given.
    public static func makeQuery(subjectType: String, subjectID: UUID, limit: Int, cursor: String? = nil) -> [URLQueryItem] {
        var query = [
            URLQueryItem(name: "subject_type", value: subjectType),
            URLQueryItem(name: "subject_id", value: subjectID.uuidString.lowercased()),
            URLQueryItem(name: "limit", value: String(limit)),
        ]
        if let cursor, !cursor.isEmpty {
            query.append(URLQueryItem(name: "cursor", value: cursor))
        }
        return query
    }

    /// One run of a kind, as a row names it: "Brief", "Tailored CV",
    /// "Company research"; nil for a kind with no name of its own here.
    public static func getRunTitle(_ kind: String) -> String? {
        switch kind {
        case "job_facts": "Job facts"
        case "job_brief": "Brief"
        case "job_brief_full": "Full brief"
        case "job_cv": "Tailored CV"
        case "recruiter_screen": "Recruiter screen"
        case "interview_prep": "Interview pack"
        case "job_fix": "Job fix"
        case "company_triage": "Company research"
        case "job_finder": "Find jobs"
        default: nil
        }
    }

    /// A row's name: one run of its kind, or the kind as the catalog names
    /// it.
    public static func getTitle(ofKind kind: String) -> String {
        getRunTitle(kind) ?? RunsSummary.getKindTitle(kind)
    }

    /// How a finished run ended, and how long it took: "Succeeded in 4.2 s",
    /// "Failed after 2 min 5 s", "Invalid answer after 3.0 s".
    public static func describeOutcome(_ run: ActivityRun) -> String {
        let outcome = switch run.outcome {
        case "succeeded": "Succeeded"
        case "running": "Running"
        case "invalid": "Invalid answer"
        default: "Failed"
        }
        guard let milliseconds = run.durationMS else { return outcome }
        let took = RunFormat.describeDuration(milliseconds: milliseconds)
        return run.outcome == "succeeded" ? "\(outcome) in \(took)" : "\(outcome) after \(took)"
    }

    /// The runs listed once the latest page is read again: the page, then
    /// the runs already loaded past its last, with the cursor they were
    /// loaded to. A page that holds every run, or whose last run isn't
    /// among those loaded, is the list.
    public static func merge(_ page: ActivityRunsPage, into shown: [ActivityRun], cursor: String) -> ActivityRunsPage {
        guard page.hasMore, let last = page.runs.last, let index = shown.firstIndex(where: { $0.id == last.id }) else { return page }
        let listed = Set(page.runs.map(\.id))
        let rest = shown[(index + 1)...].filter { !listed.contains($0.id) }
        guard !rest.isEmpty else { return page }
        return ActivityRunsPage(runs: page.runs + rest, nextCursor: cursor)
    }
}
