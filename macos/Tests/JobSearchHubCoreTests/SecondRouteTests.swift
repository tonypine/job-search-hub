import Foundation
@testable import JobSearchHubCore
import Testing

private let companyID = UUID(uuidString: "cccccccc-0000-0000-0000-000000000001")!

private let json = #"""
{"people":[
  {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000001","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000001",
   "name":"Rita Example","company_id":"cccccccc-0000-0000-0000-000000000001","company_name":"Initech","is_agency":false,
   "open_jobs":0,"fitting_jobs":0},
  {"key":"contact:eeeeeeee-0000-0000-0000-000000000001","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000001",
   "name":"Eli Example","role":"Staff Engineer","relevance":"engineer","company_id":"cccccccc-0000-0000-0000-000000000001",
   "company_name":"Initech","is_agency":false,"open_jobs":0,"fitting_jobs":0},
  {"key":"contact:eeeeeeee-0000-0000-0000-000000000002","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000002",
   "name":"Morgan Example","role":"Head of Engineering","relevance":"hiring_manager","company_id":"cccccccc-0000-0000-0000-000000000001",
   "company_name":"Initech","email":"morgan@initech.example","is_agency":false,"open_jobs":0,"fitting_jobs":0},
  {"key":"introducer:bbbbbbbb-0000-0000-0000-000000000001:cccccccc-0000-0000-0000-000000000001","relation":"introducer",
   "id":"bbbbbbbb-0000-0000-0000-000000000001","name":"Riley Example","role":"Former colleague",
   "company_id":"cccccccc-0000-0000-0000-000000000001","company_name":"Initech","note":"Interviewed there",
   "preferred_channel":"WhatsApp","is_agency":false,"open_jobs":0,"fitting_jobs":0},
  {"key":"connection:dddddddd-0000-0000-0000-000000000001","relation":"connection","id":"dddddddd-0000-0000-0000-000000000001",
   "name":"Alex Example","role":"Engineering Manager","company_id":"cccccccc-0000-0000-0000-000000000001","company_name":"Initech",
   "is_agency":false,"open_jobs":0,"fitting_jobs":0},
  {"key":"contact:eeeeeeee-0000-0000-0000-000000000003","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000003",
   "name":"Ola Example","relevance":"other","company_id":"cccccccc-0000-0000-0000-000000000001","company_name":"Initech",
   "is_agency":false,"open_jobs":0,"fitting_jobs":0},
  {"key":"contact:eeeeeeee-0000-0000-0000-000000000004","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000004",
   "name":"Gus Example","relevance":"hiring_manager","company_id":"cccccccc-0000-0000-0000-000000000002","company_name":"Globex",
   "is_agency":false,"open_jobs":0,"fitting_jobs":0}
]}
"""#

private func decodePeople() throws -> [RelatedPerson] {
    try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(json.utf8)).people
}

private func makeCard(jobTitle: String?) -> PipelineCard {
    let now = Date(timeIntervalSince1970: 1_790_600_000)
    let application = Application(id: UUID(), companyID: companyID, phaseID: UUID(), phaseEnteredAt: now, createdAt: now, updatedAt: now)
    return PipelineCard(application: application, jobTitle: jobTitle, companyName: "Initech", unseenUpdates: 0)
}

@Test func theSecondRouteOffersWarmPathsFirstThenTheContactsClosestToTheHire() throws {
    let candidates = SecondRoute.getCandidates(companyID: companyID, among: try decodePeople())

    #expect(candidates.map(\.name) == ["Alex Example", "Riley Example", "Morgan Example", "Eli Example", "Ola Example"])
    #expect(SecondRoute.getCandidates(companyID: UUID(), among: try decodePeople()).isEmpty)
}

@Test func eachSecondRouteCandidateSaysWhoTheyAre() throws {
    let people = try decodePeople()

    #expect(SecondRoute.getMenuTitle(for: people[4]) == "Alex Example · Engineering Manager, your connection")
    #expect(SecondRoute.getMenuTitle(for: people[3]) == "Riley Example · can introduce you")
    #expect(SecondRoute.getMenuTitle(for: people[2]) == "Morgan Example · Hiring manager")
    #expect(SecondRoute.getMenuTitle(for: people[5]) == "Ola Example")
}

@Test func theDraftRequestAimsTheOutreachPromptAtThePerson() throws {
    let people = try decodePeople()

    let request = SecondRoute.makeDraftRequest(prompt: "Draft a first message to someone at this company.\n", to: people[2], about: makeCard(jobTitle: "Staff Engineer"))

    #expect(request == """
        Draft a first message to someone at this company.

        Write it to Morgan Example (Head of Engineering). Hiring manager at Initech, found by company research.
        Reach them: morgan@initech.example.
        My application for Staff Engineer went out and nobody has answered it past its follow-up, so this is a second route in. \
        Draft it for me to review; I'll send it myself.
        """)
}

@Test func theDraftRequestToAnIntroducerAsksAboutTheCompany() throws {
    let introducer = try decodePeople()[3]

    let request = SecondRoute.makeDraftRequest(prompt: "Draft.", to: introducer, about: makeCard(jobTitle: nil))

    #expect(request.contains("Write it to Riley Example (Former colleague). Can introduce you at Initech: Interviewed there."))
    #expect(request.contains("Reach them: WhatsApp."))
    #expect(request.contains("My message to the company went out"))
}
