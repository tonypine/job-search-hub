import Foundation
import JobSearchHubCore
import Testing

// Shaped exactly like the hub's JSON (its Go struct tags), with made-up data:
// fixtures of real answers would put real people's names in the repository.
private let companiesJSON = ##"""
{"companies":[
  {"company":{"id":"0aa55565-58d2-4247-ba01-cba65060a316","name":"Acme","domain":"acme.com",
              "website_url":"https://www.acme.com","careers_url":"https://acme.com/careers",
              "headquarters_country":"Canada","employee_count_range":"51-200","summary":"Makes anvils.",
              "found_via":"Referral: a former colleague","created_at":"2026-09-28T12:00:00.5Z","updated_at":"2026-09-28T12:05:00Z"},
   "watched_since":"2026-09-28T12:10:00.123456Z",
   "job_boards":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                  "provider":"ashby","board_token":"acme","board_url":"https://jobs.ashbyhq.com/acme",
                  "verified_at":"2026-09-28T12:01:00Z","open_posting_count":42}],
   "people_count":4,"connection_count":2,"known_people":["Ada Example","Grace Example"],"application_phase":"Applied",
   "fitting_jobs":3,"best_match":"strong","newest_fitting_job_seen_at":"2026-09-30T09:00:00Z","unseen_updates":3},
  {"company":{"id":"1bb55565-58d2-4247-ba01-cba65060a316","name":"Zeta","domain":"zeta.com",
              "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
   "job_boards":[],"people_count":0,"connection_count":0,"unseen_updates":0}
]}
"""##

private let dossierJSON = ##"""
{"company":{"id":"0aa55565-58d2-4247-ba01-cba65060a316","name":"Acme","domain":"acme.com",
            "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
 "job_boards":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                "provider":"ashby","board_token":"acme","board_url":"https://jobs.ashbyhq.com/acme",
                "verified_at":"2026-09-28T12:01:00Z"}],
 "people":[{"id":"2cc55565-58d2-4247-ba01-cba65060a316","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
            "name":"Ada Lovelace","role_title":"Engineering Manager","relevance":"hiring_manager",
            "source_url":"https://acme.com/team","email":"ada@acme.com","created_at":"2026-09-28T12:02:00Z"}],
 "connections":[{"id":"3dd55565-58d2-4247-ba01-cba65060a316","first_name":"Grace","last_name":"Hopper",
                 "profile_url":"https://www.linkedin.com/in/grace-example","company_name":"Acme","position":"Senior Engineer",
                 "connected_on":"2026-10-01T00:00:00Z","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                 "imported_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"}]}
"""##

@Test func theCompaniesListDecodes() throws {
    let listed = try HubJSON.makeDecoder().decode(CompaniesResponse.self, from: Data(companiesJSON.utf8)).companies

    #expect(listed.count == 2)
    let acme = listed[0]
    #expect(acme.company.websiteURL == "https://www.acme.com")
    #expect(acme.company.careersURL == "https://acme.com/careers")
    #expect(acme.company.foundVia == "Referral: a former colleague")
    #expect(acme.watchedSince != nil)
    #expect(acme.jobBoards.first?.summaryLine == "ashby/acme · 42 open")
    #expect(acme.peopleCount == 4 && acme.unseenUpdates == 3 && acme.connectionCount == 2)
    #expect(acme.knownPeople == ["Ada Example", "Grace Example"] && acme.applicationPhase == "Applied")
    #expect(acme.fittingJobs == 3 && acme.bestMatch == .strong && acme.newestFittingJobSeenAt != nil)

    let zeta = listed[1]
    #expect(zeta.company.websiteURL == nil)
    #expect(zeta.watchedSince == nil)
    #expect(zeta.jobBoards.isEmpty)
    #expect(zeta.knownPeople == nil && zeta.fittingJobs == nil && zeta.bestMatch == nil)
}

@Test func aDossierDecodesWithPeopleAndAnUncountedBoard() throws {
    let dossier = try HubJSON.makeDecoder().decode(CompanyDossier.self, from: Data(dossierJSON.utf8))

    #expect(dossier.jobBoards.first?.summaryLine == "ashby/acme · open postings unknown")
    #expect(dossier.people.first?.sourceURL == "https://acme.com/team")
    #expect(dossier.people.first?.roleTitle == "Engineering Manager")
    #expect(dossier.people.first?.profileURL == nil)
    #expect(dossier.people.first?.email == "ada@acme.com")
    let grace = try #require(dossier.connections?.first)
    #expect(grace.fullName == "Grace Hopper" && grace.connectedSince == "Connected since Oct 2026")
}

private let companyPageJSON = ##"""
{"company":{"id":"0aa55565-58d2-4247-ba01-cba65060a316","name":"Acme","domain":"acme.com",
            "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
 "job_boards":[],"people":[],"connections":[],"warm_paths":[],
 "applications":[
   {"application":{"id":"4ee55565-58d2-4247-ba01-cba65060a316","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                   "phase_id":"5ff55565-58d2-4247-ba01-cba65060a316","closed_reason":"No answer.",
                   "phase_entered_at":"2026-09-20T12:00:00Z","created_at":"2026-09-01T12:00:00Z","updated_at":"2026-09-20T12:00:00Z"},
    "company_name":"Acme","unseen_updates":0,"phase_name":"Closed","phase_is_closed":true},
   {"application":{"id":"6aa55565-58d2-4247-ba01-cba65060a316","job_id":"7bb55565-58d2-4247-ba01-cba65060a316",
                   "company_id":"0aa55565-58d2-4247-ba01-cba65060a316","phase_id":"8cc55565-58d2-4247-ba01-cba65060a316",
                   "phase_entered_at":"2026-09-28T12:00:00Z","created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
    "job_title":"Engineer","job_url":"https://acme.com/jobs/1","company_name":"Acme",
    "follow_up_due_at":"2026-10-05T12:00:00Z","unseen_updates":1,"phase_name":"Applied","phase_is_closed":false}],
 "mail":[
   {"id":"9dd55565-58d2-4247-ba01-cba65060a316","gmail_message_id":"m2","thread_id":"t2","direction":"received",
    "sender":"jobs@acme.com","recipients":"owner@example.com","subject":"Interview with Acme",
    "sent_at":"2026-10-02T09:00:00Z","label_ids":["INBOX"],"recorded_at":"2026-10-02T09:01:00Z",
    "company_id":"0aa55565-58d2-4247-ba01-cba65060a316","matched_by":"domain","classification":"interview_invite"},
   {"id":"1ee55565-58d2-4247-ba01-cba65060a316","gmail_message_id":"m1","thread_id":"t1","direction":"sent",
    "sender":"owner@example.com","recipients":"jobs@acme.com","subject":"Following up",
    "sent_at":"2026-10-01T09:00:00Z","label_ids":["SENT"],"recorded_at":"2026-10-01T09:01:00Z",
    "company_id":"0aa55565-58d2-4247-ba01-cba65060a316","matched_by":"thread"}],
 "folded_mail_count":3}
"""##

@Test func aCompanyPageDecodesWithItsApplicationsAndMail() throws {
    let dossier = try HubJSON.makeDecoder().decode(CompanyDossier.self, from: Data(companyPageJSON.utf8))

    let applications = dossier.applicationsOpenFirst
    #expect(applications.map(\.phaseName) == ["Applied", "Closed"])
    #expect(applications[0].card.jobTitle == "Engineer" && applications[0].card.followUpDueAt != nil && !applications[0].phaseIsClosed)
    #expect(applications[1].card.jobTitle == nil && applications[1].card.application.closedReason == "No answer." && applications[1].phaseIsClosed)

    let mail = try #require(dossier.mail)
    #expect(mail.map(\.subject) == ["Interview with Acme", "Following up"])
    #expect(mail[0].fromLine == "jobs@acme.com · interview invite")
    #expect(mail[1].fromLine == "You wrote")
    #expect(dossier.foldedMailLine == "3 newsletters and job alerts left out")
}

@Test func aDossierFromAHubWithoutThreadsHasNone() throws {
    let dossier = try HubJSON.makeDecoder().decode(CompanyDossier.self, from: Data(dossierJSON.utf8))

    #expect(dossier.applications == nil && dossier.mail == nil && dossier.foldedMailLine == nil)
    #expect(dossier.applicationsOpenFirst.isEmpty)
}
