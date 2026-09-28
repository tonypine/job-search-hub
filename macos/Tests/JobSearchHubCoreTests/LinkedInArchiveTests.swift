import Foundation
@testable import JobSearchHubCore
import Testing

@Test func anExportsKnownFilesAreImportedConnectionsFirst() {
    let folder = URL(fileURLWithPath: "/tmp/export")
    let files = ["messages.csv", "Skills.csv", "Invitations.csv", "Connections.csv"].map { folder.appending(path: $0) }

    let imports = LinkedInArchive.findImports(in: files)

    #expect(imports.map(\.kind) == [.connections, .messages, .invitations])
    #expect(imports.first?.kind.importPath == "v1/connections/import")
}

@Test func importResultsSayWhatTheyStored() throws {
    let decoder = HubJSON.makeDecoder()
    let messages = try decoder.decode(MessagesImport.self, from: Data(#"{"conversations":444,"messages":2056,"connections_with_history":207}"#.utf8))
    #expect(messages.summary == "Conversations: 444 (2056 messages); 207 connections you've talked with.")
    let invitations = try decoder.decode(InvitationsImport.self, from: Data(#"{"incoming":10,"outgoing":10}"#.utf8))
    #expect(invitations.summary == "Invitations: 10 received, 10 sent.")
}
