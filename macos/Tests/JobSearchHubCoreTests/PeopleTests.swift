import Foundation
import JobSearchHubCore
import Testing

private let json = #"""
{"people":[
  {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000001","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000001",
   "name":"Rita Example","role":"Talent Partner","company_id":"cccccccc-0000-0000-0000-000000000001","company_name":"Initech",
   "profile_url":"https://www.linkedin.com/in/rita-example","hiring_role":"Engineer","is_agency":false,
   "last_contact_at":"2026-10-01T10:00:00.123456Z","answered":false,"open_jobs":3,"fitting_jobs":2},
  {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000002","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000002",
   "name":"Sam Example","is_agency":true,"last_contact_at":"2026-09-28T10:00:00Z","answered":true,"open_jobs":0,"fitting_jobs":0},
  {"key":"introducer:bbbbbbbb-0000-0000-0000-000000000001:cccccccc-0000-0000-0000-000000000002","relation":"introducer",
   "id":"bbbbbbbb-0000-0000-0000-000000000001","name":"Riley Example","role":"Former colleague","how_known":"Former colleague",
   "company_id":"cccccccc-0000-0000-0000-000000000002","company_name":"Northwind","note":"Interviewed there","is_agency":false,
   "open_jobs":1,"fitting_jobs":0},
  {"key":"connection:dddddddd-0000-0000-0000-000000000001","relation":"connection","id":"dddddddd-0000-0000-0000-000000000001",
   "name":"Alex Example","role":"Engineering Manager","company_id":"cccccccc-0000-0000-0000-000000000002","company_name":"Northwind",
   "closeness":"never talked","is_agency":false,"open_jobs":1,"fitting_jobs":0},
  {"key":"contact:eeeeeeee-0000-0000-0000-000000000001","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000001",
   "name":"Morgan Example","role":"Head of Engineering","relevance":"hiring_manager","company_id":"cccccccc-0000-0000-0000-000000000003",
   "company_name":"Globex","source_url":"https://globex.example/team","is_agency":false,"open_jobs":0,"fitting_jobs":0}
]}
"""#

private func decodePeople() throws -> [RelatedPerson] {
    try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(json.utf8)).people
}

@Test func peopleOfEveryRelationDecode() throws {
    let people = try decodePeople()

    #expect(people.map(\.relation) == [.recruiter, .recruiter, .introducer, .connection, .contact])
    #expect(Set(people.map(\.id)).count == people.count)
    #expect(people[0].conversationID == people[0].personID && people[2].conversationID == nil)
    #expect(people[0].isUnanswered && !people[1].isUnanswered && !people[2].isUnanswered)
    #expect(people[0].lastContactAt != nil && people[4].sourceURL == "https://globex.example/team")
    #expect(people[0].reference == PersonReference(key: people[0].key, companyID: people[0].companyID))
}

@Test func peopleFilterByRelationHiringAndAnswer() throws {
    let people = try decodePeople()

    #expect(PeopleFilter().apply(to: people).count == 5)
    #expect(PeopleFilter(relation: .recruiter).apply(to: people).map(\.name) == ["Rita Example", "Sam Example"])
    #expect(PeopleFilter(hiringNowOnly: true).apply(to: people).map(\.name) == ["Rita Example", "Riley Example", "Alex Example"])
    #expect(PeopleFilter(unansweredOnly: true).apply(to: people).map(\.name) == ["Rita Example"])
    #expect(PeopleFilter(relation: .connection, unansweredOnly: true).apply(to: people).isEmpty)
}

@Test func peopleAreCountedWithTheirCompanies() throws {
    let people = try decodePeople()

    #expect(PeopleFilter.describeCounts(people) == "5 people at 3 companies")
    #expect(PeopleFilter.describeCounts([people[1]]) == "1 person")
    #expect(PersonRelation.allCases.map(\.pluralTitle) == ["Contacts", "Connections", "Introducers", "Recruiters"])
}

@Test func eachRelationSaysWhatTheyCanDo() throws {
    let people = try decodePeople()

    #expect(people[0].whatTheyCanDo == "Recruits for Initech. The role: Engineer. Their company has 3 open jobs, and 2 fit you.")
    #expect(people[1].whatTheyCanDo == "Recruits through an agency.")
    #expect(people[1].relationTitle == "Agency recruiter" && people[0].relationTitle == "Recruiter")
    #expect(people[2].whatTheyCanDo == "Can introduce you at Northwind: Interviewed there. Their company has 1 open job.")
    #expect(people[3].whatTheyCanDo == "A LinkedIn connection at Northwind: never talked. Their company has 1 open job.")
    #expect(people[4].whatTheyCanDo == "Hiring manager at Globex, found by company research.")
    #expect(people[0].openingsText == "3 open, 2 pass the screen" && people[4].openingsText.isEmpty)
}

@Test func thePeopleQueryNarrowsToACompany() {
    let id = UUID()
    #expect(PeopleQuery.make(companyID: nil).isEmpty)
    #expect(PeopleQuery.make(companyID: id) == [URLQueryItem(name: "company_id", value: id.uuidString)])
}
