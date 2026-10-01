import Foundation
@testable import JobSearchHubCore
import Testing

@Test func aMarketGapReadsWithItsDemandAndPlan() throws {
    let json = #"{"gaps":[{"technology":"AWS","job_count":14,"good_fits":43,"job_ids":[],"plan_kind":"learn","plan":"Take the course.","computed_at":"2026-09-30T23:12:31.8Z"},"#
        + #"{"technology":"Docker","job_count":8,"good_fits":43,"job_ids":[],"plan_kind":"portfolio","plan":"Containerize it.","computed_at":"2026-09-30T23:12:31.8Z"}]}"#
    let gaps = try HubJSON.makeDecoder().decode(MarketGapsResponse.self, from: Data(json.utf8)).gaps
    #expect(gaps.map(\.demandText) == ["Asked for by 14 of 43 good fits", "Asked for by 8 of 43 good fits"])
    #expect(gaps.map(\.planTitle) == ["Learn", "Show it"])
}
