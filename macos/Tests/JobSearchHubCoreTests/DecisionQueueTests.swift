import Foundation
@testable import JobSearchHubCore
import Testing

@Test func theQueueAndTheSignalsDecode() throws {
    let queueJSON = #"""
    {"items":[{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Frontend Engineer","url":"https://acme.com/1",
               "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
               "company_name":"Acme","match":"strong","reason":"React.","brief_tier":"pre","fit":{"level":"good","checks":[]}}],"total":1}
    """#
    let queue = try HubJSON.makeDecoder().decode(DecisionQueueResponse.self, from: Data(queueJSON.utf8))
    #expect(queue.items.first?.match == .strong && queue.items.first?.companyName == "Acme" && queue.items.first?.decision == nil)

    let signalsJSON = #"{"since":"2026-09-23T20:51:42.7Z","decisions":{"pursue":1,"skip":2,"later":0},"median_hours_to_decide":36,"good_or_unclear_seen":127,"good_or_unclear_decided":9}"#
    let signals = try HubJSON.makeDecoder().decode(DecisionSignals.self, from: Data(signalsJSON.utf8))
    #expect(signals.summary == "This week: 1 pursued, 2 skipped, 0 for later · median 36 hours to decide · 9 of 127 jobs that don't fail the screen decided")
}

@Test func waitsReadAsPeopleSayThem() {
    #expect(DecisionSignals.formatWait(0.5) == "30 minutes")
    #expect(DecisionSignals.formatWait(5.2) == "5 hours")
    #expect(DecisionSignals.formatWait(72) == "3 days")
    #expect(DecisionSignals.formatWait(1.0 / 60) == "1 minute")
    #expect(DecisionSignals.formatWait(1.2) == "1 hour")
}
