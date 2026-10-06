import Foundation
@testable import JobSearchHubCore
import Testing

@Test func aConnectionsImportDecodesAndSaysWhatItDid() throws {
    let json = #"{"added":120,"updated":3,"matched":4,"skipped":1}"#
    let imported = try HubJSON.makeDecoder().decode(ConnectionsImport.self, from: Data(json.utf8))
    #expect(imported.summary == "Imported: 120 added, 3 updated, 1 skipped without a profile URL. 4 work at companies in the hub.")
    let summary = try HubJSON.makeDecoder().decode(ConnectionsSummary.self, from: Data(#"{"count":123,"matched":4,"conversations":0,"invitations":0}"#.utf8))
    #expect(summary.lastImportedAt == nil && summary.matched == 4)
}

@Test func aSuggestionSaysWhyItsWorthResearching() throws {
    let decoder = HubJSON.makeDecoder()
    let hiring = try decoder.decode(CompanySuggestion.self, from: Data(#"{"organization":"Globex","connection_count":1,"open_jobs":3,"fitting_jobs":2}"#.utf8))
    let quiet = try decoder.decode(CompanySuggestion.self, from: Data(#"{"organization":"Initech","followed_at":"2019-05-01T12:00:00Z","connection_count":0,"open_jobs":0,"fitting_jobs":0}"#.utf8))
    #expect(hiring.reason == "2 open jobs pass the screen · 1 person you know")
    #expect(quiet.reason == "Followed since 2019")
    #expect(quiet.origin == "Followed on LinkedIn" && quiet.researchTarget == "Initech")
}

@Test func aGallerySuggestionSaysWhereItCameFromAndResearchesItsSite() throws {
    let json = #"{"organization":"Hooli","source":"startups_gallery","website":"https://www.hooli.example/","careers_url":"https://jobs.ashbyhq.com/hooli","connection_count":0,"open_jobs":3,"fitting_jobs":1}"#
    let suggestion = try HubJSON.makeDecoder().decode(CompanySuggestion.self, from: Data(json.utf8))
    #expect(suggestion.source == .startupsGallery && suggestion.careersURL == "https://jobs.ashbyhq.com/hooli")
    #expect(suggestion.reason == "1 open job passes the screen")
    #expect(suggestion.origin == "On startups.gallery's remote list")
    #expect(suggestion.researchTarget == "hooli.example")
}
