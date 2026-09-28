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
   "people_count":4,"unseen_updates":3},
  {"company":{"id":"1bb55565-58d2-4247-ba01-cba65060a316","name":"Zeta","domain":"zeta.com",
              "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
   "job_boards":[],"people_count":0,"unseen_updates":0}
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
            "source_url":"https://acme.com/team","email":"ada@acme.com","created_at":"2026-09-28T12:02:00Z"}]}
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
    #expect(acme.peopleCount == 4 && acme.unseenUpdates == 3)

    let zeta = listed[1]
    #expect(zeta.company.websiteURL == nil)
    #expect(zeta.watchedSince == nil)
    #expect(zeta.jobBoards.isEmpty)
}

@Test func aDossierDecodesWithPeopleAndAnUncountedBoard() throws {
    let dossier = try HubJSON.makeDecoder().decode(CompanyDossier.self, from: Data(dossierJSON.utf8))

    #expect(dossier.jobBoards.first?.summaryLine == "ashby/acme · open postings unknown")
    #expect(dossier.people.first?.sourceURL == "https://acme.com/team")
    #expect(dossier.people.first?.roleTitle == "Engineering Manager")
    #expect(dossier.people.first?.profileURL == nil)
    #expect(dossier.people.first?.email == "ada@acme.com")
}
