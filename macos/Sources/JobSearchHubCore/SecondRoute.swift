import Foundation

/// The next route for an application nobody answered past its follow-up:
/// someone at the company, or someone who can introduce the owner there.
public enum SecondRoute {
    /// Contacts closest to the hire first; any other relevance comes after.
    private static let contactRelevanceOrder = ["hiring_manager", "recruiter", "engineering_lead", "founder", "engineer"]

    /// The people at the company worth writing to, best first: the owner's
    /// connections who work there and those who can introduce them, then the
    /// contacts company research found, the ones closest to the hire first.
    /// Recruiters who wrote about other roles are left out. People of the
    /// same rank keep the order they came in.
    public static func getCandidates(companyID: UUID, among people: [RelatedPerson]) -> [RelatedPerson] {
        people.enumerated()
            .filter { $0.element.companyID == companyID }
            .compactMap { entry in getRank(of: entry.element).map { (rank: $0, index: entry.offset, person: entry.element) } }
            .sorted { ($0.rank, $0.index) < ($1.rank, $1.index) }
            .map(\.person)
    }

    private static func getRank(of person: RelatedPerson) -> Int? {
        switch person.relation {
        case .connection: 0
        case .introducer: 1
        case .contact: 2 + (contactRelevanceOrder.firstIndex(of: person.relevance ?? "") ?? contactRelevanceOrder.count)
        case .recruiter: nil
        }
    }

    /// The person in the Write to menu: "Morgan Example · Hiring manager".
    public static func getMenuTitle(for person: RelatedPerson) -> String {
        let role = person.role.flatMap { $0.isEmpty ? nil : $0 }
        let detail: String? = switch person.relation {
        case .introducer: "can introduce you"
        case .connection: [role, "your connection"].compactMap { $0 }.joined(separator: ", ")
        case .contact, .recruiter: person.relevanceTitle ?? role
        }
        return detail.map { "\(person.name) · \($0)" } ?? person.name
    }

    /// The stored outreach_draft prompt aimed at one person: who they are,
    /// where to reach them, and why the message goes out, so the session
    /// drafts it for them and leaves the sending to the owner.
    public static func makeDraftRequest(prompt: String, to person: RelatedPerson, about card: PipelineCard) -> String {
        let role = person.role.flatMap { $0.isEmpty ? nil : " (\($0))" } ?? ""
        var lines = [prompt.trimmingCharacters(in: .whitespacesAndNewlines), "", "Write it to \(person.name)\(role). \(person.whatTheyCanDo)"]
        let reach = [person.email, person.preferredChannel, person.profileURL].compactMap { $0?.isEmpty == false ? $0 : nil }
        if !reach.isEmpty {
            lines.append("Reach them: \(reach.joined(separator: ", ")).")
        }
        let application = card.jobTitle.map { "My application for \($0)" } ?? "My message to the company"
        lines.append(
            "\(application) went out and nobody has answered it past its follow-up, so this is a second route in. "
                + "Draft it for me to review; I'll send it myself."
        )
        return lines.joined(separator: "\n")
    }
}
