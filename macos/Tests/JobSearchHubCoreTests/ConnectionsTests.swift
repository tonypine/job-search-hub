import Foundation
@testable import JobSearchHubCore
import Testing

@Test func aConnectionsImportDecodesAndSaysWhatItDid() throws {
    let json = #"{"added":120,"updated":3,"matched":4,"skipped":1}"#
    let imported = try HubJSON.makeDecoder().decode(ConnectionsImport.self, from: Data(json.utf8))
    #expect(imported.summary == "Imported: 120 added, 3 updated, 1 skipped without a profile URL. 4 work at companies in the hub.")
    let summary = try HubJSON.makeDecoder().decode(ConnectionsSummary.self, from: Data(#"{"count":123,"matched":4}"#.utf8))
    #expect(summary.lastImportedAt == nil && summary.matched == 4)
}
