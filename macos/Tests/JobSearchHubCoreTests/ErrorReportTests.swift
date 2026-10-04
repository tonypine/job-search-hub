import Foundation
@testable import JobSearchHubCore
import Testing

@Test func aStoppedHubSaysToTryAgainAndKeepsTheRawError() {
    let report = ErrorReport(HubError.unreachable("Could not connect to the server."))

    #expect(report.advice.contains("didn't answer"))
    #expect(report.details?.contains("Could not connect to the server.") == true)
}

@Test func aRefusedTokenPointsToSettings() {
    #expect(ErrorReport(HubError.unauthorized).advice.contains("Settings"))
}

@Test func theHubsOwnWordsShowForARequestItTurnsDown() {
    let report = ErrorReport(HubError.server(status: 400, message: "url is required"))

    #expect(report.advice == "The hub turned it down: url is required")
}

@Test func aServerFailureDoesNotShowItsMessageAsAdvice() {
    let report = ErrorReport(HubError.server(status: 500, message: "pq: relation does not exist"))

    #expect(!report.advice.contains("pq:"))
    #expect(report.details?.contains("pq: relation does not exist") == true)
}

@Test func everyHubErrorHasAdvice() {
    let errors: [HubError] = [
        .unreachable(""), .unauthorized, .forbidden, .notFound,
        .server(status: 409, message: ""), .server(status: 502, message: "bad gateway"), .undecodable(""),
    ]
    #expect(errors.allSatisfy { !ErrorReport($0).advice.isEmpty })
}

@Test func otherErrorsStillReportTheirDetails() {
    struct Odd: Error {}
    let report = ErrorReport(Odd())

    #expect(!report.advice.isEmpty)
    #expect(report.details?.contains("Odd") == true)
}

@Test func aPlainReportHasNoDetails() {
    #expect(ErrorReport(advice: "Pick a file first.").details == nil)
}
