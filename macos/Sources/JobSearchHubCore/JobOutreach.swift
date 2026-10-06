import Foundation

/// The people a job's Overview names, and the message to each: someone at
/// the company gets a message about the job, someone who can introduce the
/// owner is asked for the introduction.
public enum JobOutreach {
    /// The most people the Overview lists; All opens the rest.
    public static let shownLimit = 3

    /// The people worth writing to about the job, best first: connections,
    /// introducers, then the contacts closest to the hire.
    public static func getPeople(companyID: UUID, among people: [RelatedPerson]) -> [RelatedPerson] {
        SecondRoute.getCandidates(companyID: companyID, among: people)
    }

    /// Whether the owner knows them: a connection or an introducer.
    public static func isKnown(_ person: RelatedPerson) -> Bool {
        person.relation == .connection || person.relation == .introducer
    }

    /// The action that fits them: "Ask for intro" for an introducer, else
    /// "Message".
    public static func getActionTitle(for person: RelatedPerson) -> String {
        person.relation == .introducer ? "Ask for intro" : "Message"
    }

    /// The stored outreach_draft prompt aimed at one person about a job, so
    /// the job's session drafts it for them and leaves the sending to the
    /// owner.
    public static func makeDraftRequest(prompt: String, to person: RelatedPerson, aboutJob title: String, at companyName: String?) -> String {
        let role = person.role.flatMap { $0.isEmpty ? nil : " (\($0))" } ?? ""
        var lines = [prompt.trimmingCharacters(in: .whitespacesAndNewlines), "", "Write it to \(person.name)\(role). \(person.whatTheyCanDo)"]
        let reach = [person.email, person.preferredChannel, person.profileURL].compactMap { $0?.isEmpty == false ? $0 : nil }
        if !reach.isEmpty {
            lines.append("Reach them: \(reach.joined(separator: ", ")).")
        }
        let job = companyName.map { "the \(title) role at \($0)" } ?? "the \(title) role"
        let ask = person.relation == .introducer
            ? "Ask them to introduce me to the people hiring for \(job)."
            : "I'm looking at \(job) and want to reach them about it before or as I apply."
        lines.append(ask + " Draft it for me to review; I'll send it myself.")
        return lines.joined(separator: "\n")
    }
}

public extension RelatedPerson {
    /// A connection at the job's company as the people list has them, for
    /// while the list isn't read.
    init(_ connection: Connection, companyID: UUID?, companyName: String?) {
        key = "\(PersonRelation.connection.rawValue):\(connection.id.uuidString.lowercased())"
        relation = .connection
        personID = connection.id
        name = connection.fullName
        role = connection.position
        self.companyID = companyID
        self.companyName = companyName ?? connection.companyName
        profileURL = connection.profileURL
        email = connection.email
        closeness = connection.closeness ?? connection.connectedSince
        isAgency = false
        openJobs = 0
        fittingJobs = 0
    }
}
