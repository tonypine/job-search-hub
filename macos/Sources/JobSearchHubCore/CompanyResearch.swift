import Foundation

/// The `hub` command bundled inside the app, which runs Claude on this Mac
/// for the work the hub can't do in its container, and reports to the hub as
/// the owner.
public enum BundledHubCommand {
    /// The command's environment: the same short list a session gets, plus
    /// the hub it reports to and the `claude` it starts.
    public static func makeEnvironment(from inherited: [String: String], hubURL: URL, ownerToken: String, claude: String) -> [String: String] {
        var environment = inherited.filter { SessionEnvironment.passedVariables.contains($0.key) }
        environment["PATH"] = SessionEnvironment.basePath
        environment["HUB_URL"] = hubURL.absoluteString
        environment["HUB_OWNER_TOKEN"] = ownerToken
        environment["HUB_CLAUDE_BIN"] = claude
        return environment
    }
}

/// How the app runs the company triage agent: `hub company add`, which prints
/// its progress line by line.
public enum CompanyResearchLaunch {
    /// The arguments of `hub company add`.
    public static func makeArguments(company: String, foundVia: String) -> [String] {
        var arguments = ["company", "add", company.trimmingCharacters(in: .whitespacesAndNewlines)]
        let note = foundVia.trimmingCharacters(in: .whitespacesAndNewlines)
        if !note.isEmpty {
            arguments += ["--found-via", note]
        }
        return arguments
    }

    /// The company a finished run added, from its last line,
    /// "Company id: <uuid>".
    public static func parseCompanyID(_ line: String) -> UUID? {
        guard line.hasPrefix("Company id: ") else { return nil }
        return UUID(uuidString: String(line.dropFirst("Company id: ".count)).trimmingCharacters(in: .whitespaces))
    }
}

/// How the app drafts a reply to a recruiter: `hub recruiter reply`, which
/// prints the draft.
public enum RecruiterReplyLaunch {
    public static func makeArguments(conversationID: UUID) -> [String] {
        ["recruiter", "reply", conversationID.uuidString.lowercased()]
    }
}

/// How the app audits the LinkedIn profile: `hub profile audit`, which prints
/// the audit as Markdown.
public enum ProfileAuditLaunch {
    public static let arguments = ["profile", "audit"]
}

