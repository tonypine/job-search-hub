import Foundation
@testable import JobSearchHubCore
import Testing

@Test func eventsAreReadLineByLineAndHeartbeatsSkipped() {
    var parser = ServerSentEventParser()
    let lines = [": connected", "", "id: 7", "event: update", #"data: {"title":"Acme replied"}"#, "", ": heartbeat", "", "data: one", "data: two", ""]

    let events = lines.compactMap { parser.consume($0) }

    #expect(events == [
        ServerSentEvent(id: "7", name: "update", data: #"{"title":"Acme replied"}"#),
        ServerSentEvent(id: nil, name: "message", data: "one\ntwo"),
    ])
}

@Test func anUpdateDecodesAndNamesItsSubject() throws {
    let json = #"{"id":"aaaaaaaa-0000-0000-0000-000000000001","sequence":12,"kind":"reply","title":"Acme replied","job_id":"bbbbbbbb-0000-0000-0000-000000000002","company_id":"cccccccc-0000-0000-0000-000000000003","created_at":"2026-09-28T19:00:00.5Z","job_title":"Engineer","company_name":"Acme"}"#

    let update = try HubJSON.makeDecoder().decode(HubUpdate.self, from: Data(json.utf8))

    #expect(update.sequence == 12 && update.isUnseen && update.subject == "Engineer · Acme")
    let encoded = String(decoding: try HubJSON.makeEncoder().encode(UpdateSelection(jobID: update.jobID)), as: UTF8.self)
    #expect(encoded == #"{"job_id":"BBBBBBBB-0000-0000-0000-000000000002"}"#)
}

@Test func streamLinesKeepTheBlankLinesThatEndEvents() async throws {
    let bytes = AsyncStream<UInt8> { continuation in
        for byte in "id: 1\nevent: update\ndata: {}\n\n: ping\r\n\r\npartial".utf8 {
            continuation.yield(byte)
        }
        continuation.finish()
    }
    var lines: [String] = []

    for try await line in EventStreamLines(bytes) {
        lines.append(line)
    }

    #expect(lines == ["id: 1", "event: update", "data: {}", "", ": ping", ""])
}

@Test func updatesGroupByTheLocalDayTheyWereRecorded() throws {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = try #require(TimeZone(identifier: "America/Sao_Paulo"))
    func makeUpdate(_ title: String, at createdAt: String) throws -> HubUpdate {
        let json = #"{"id":"\#(UUID().uuidString)","sequence":1,"kind":"reply","title":"\#(title)","created_at":"\#(createdAt)"}"#
        return try HubJSON.makeDecoder().decode(HubUpdate.self, from: Data(json.utf8))
    }
    let updates = [
        try makeUpdate("after midnight", at: "2026-09-29T04:00:00Z"),
        try makeUpdate("before midnight", at: "2026-09-29T02:00:00Z"),
        try makeUpdate("morning", at: "2026-09-28T12:00:00Z"),
    ]

    let days = UpdateDay.makeDays(from: updates, calendar: calendar)

    #expect(days.map { $0.updates.map(\.title) } == [["after midnight"], ["before midnight", "morning"]])
}
