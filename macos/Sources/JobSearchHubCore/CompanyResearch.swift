import Foundation

/// How the app runs the company triage agent: the `hub` command bundled
/// inside the app, which starts `claude` on this Mac, reports to the hub as
/// the owner, and prints its progress line by line.
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

    /// The company a finished run added, from its last line,
    /// "Company id: <uuid>".
    public static func parseCompanyID(_ line: String) -> UUID? {
        guard line.hasPrefix("Company id: ") else { return nil }
        return UUID(uuidString: String(line.dropFirst("Company id: ".count)).trimmingCharacters(in: .whitespaces))
    }
}
