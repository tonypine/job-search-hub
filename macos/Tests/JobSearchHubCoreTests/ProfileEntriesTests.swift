import Foundation
@testable import JobSearchHubCore
import Testing

private func makeEntry(_ title: String, kind: String, role: ProfileEntry? = nil, start: String = "", end: String = "", confirmed: Bool = false) -> ProfileEntry {
    ProfileEntry(
        id: UUID(), kind: kind, roleID: role?.id, title: title, body: "", organization: "", startMonth: start, endMonth: end, skills: [],
        outcome: "", source: "cv", sourceDetail: "", confirmedAt: confirmed ? Date() : nil, updatedAt: Date()
    )
}

@Test func rolesComeCurrentFirstThenLatestWithTheirEntries() {
    let older = makeEntry("Engineer at Beta", kind: "role", start: "2019-01", end: "2021-06")
    let current = makeEntry("Senior at Acme", kind: "role", start: "2024-01")
    let latestEnded = makeEntry("Engineer at Gamma", kind: "role", start: "2021-06", end: "2023-12", confirmed: true)
    let skill = makeEntry("React", kind: "skill", role: latestEnded)
    let laterCase = makeEntry("Checkout", kind: "case", role: latestEnded, start: "2023-01")
    let earlierCase = makeEntry("Search", kind: "case", role: latestEnded, start: "2022-01")
    let unlinkedSkill = makeEntry("Figma", kind: "skill")
    let fact = makeEntry("Portuguese", kind: "fact")

    let groups = ProfileEntryGroups(entries: [older, skill, fact, current, earlierCase, unlinkedSkill, latestEnded, laterCase])

    #expect(groups.roles.map(\.role.title) == ["Senior at Acme", "Engineer at Gamma", "Engineer at Beta"])
    #expect(groups.roles[1].entries.map(\.title) == ["Checkout", "Search", "React"])
    #expect(groups.roles[1].unconfirmedIDs == [laterCase.id, earlierCase.id, skill.id])
    #expect(groups.others.map(\.kind) == ["skill", "fact"])
    #expect(groups.others.map(\.title) == ["Skills", "Facts"])
}

@Test func anEntryOfAMissingRoleIsListedByItsKind() {
    let orphan = ProfileEntry(
        id: UUID(), kind: "case", roleID: UUID(), title: "Old work", body: "", organization: "", startMonth: "", endMonth: "", skills: [],
        outcome: "", source: "cv", sourceDetail: "", confirmedAt: nil, updatedAt: Date()
    )
    #expect(ProfileEntryGroups(entries: [orphan]).others.first?.entries == [orphan])
}

@Test func anEntryInputEncodesItsRoleAsRoleID() throws {
    let roleID = UUID()
    let encoded = try HubJSON.makeEncoder().encode(ProfileEntryInput(kind: "case", roleID: roleID, title: "Checkout", startMonth: "2023-01"))
    let object = try #require(try JSONSerialization.jsonObject(with: encoded) as? [String: Any])
    #expect(object["role_id"] as? String == roleID.uuidString)
    #expect(object["start_month"] as? String == "2023-01")
    #expect(object["source"] as? String == "owner")
}

@Test func monthsReadAsARange() {
    #expect(makeEntry("Acme", kind: "role", start: "2024-01").monthsText == "2024-01 – now")
    #expect(makeEntry("Beta", kind: "role", start: "2019-01", end: "2021-06").monthsText == "2019-01 – 2021-06")
    #expect(makeEntry("Course", kind: "education").monthsText == "")
}
