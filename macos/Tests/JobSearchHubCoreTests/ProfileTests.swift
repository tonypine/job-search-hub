import Foundation
import JobSearchHubCore
import Testing

@Test func markdownBecomesHeadingsBulletsAndParagraphs() {
    let blocks = MarkdownBlocks.parse("""
    # Tony
    Senior engineer.
    Remote.

    ## Looking for
    - Remote roles
    * Yearly salary
    #not a heading
    """)

    #expect(blocks == [
        .heading(level: 1, text: "Tony"),
        .paragraph("Senior engineer.\nRemote."),
        .heading(level: 2, text: "Looking for"),
        .bullet("Remote roles"),
        .bullet("Yearly salary"),
        .paragraph("#not a heading"),
    ])
}

@MainActor
struct ProfileEditorTests {
    let hubURL = URL(string: "http://localhost:8090")!

    @Test func aSaveSendsTheDraftAndShowsWhatTheHubStored() async throws {
        let (session, recording) = StubHub.makeSession(answers: [
            "/v1/profile": .init(status: 200, body: ##"{"body":"# Saved by the hub","updated_at":"2026-09-28T12:00:00Z"}"##),
        ])
        let editor = ProfileEditor()
        editor.startEditing()
        editor.draft = "# Draft"

        await editor.save(with: HubClient(baseURL: hubURL, token: "owner-token", session: session))

        #expect(recording.lastRequest?.httpMethod == "PUT")
        let sent = try JSONDecoder().decode([String: String].self, from: try #require(recording.lastBody))
        #expect(sent == ["body": "# Draft"])
        #expect(editor.profile?.body == "# Saved by the hub")
        #expect(!editor.isEditing && !editor.isSaving && editor.errorMessage == nil)
    }

    @Test func aFailedSaveKeepsTheEditAndSaysWhy() async {
        let (session, _) = StubHub.makeSession(answers: [
            "/v1/profile": .init(status: 500, body: #"{"error":"database down"}"#),
        ])
        let editor = ProfileEditor()
        editor.startEditing()
        editor.draft = "# Draft"

        await editor.save(with: HubClient(baseURL: hubURL, token: "owner-token", session: session))

        #expect(editor.isEditing)
        #expect(editor.draft == "# Draft")
        #expect(editor.errorMessage?.contains("database down") == true)
    }
}

@Test func theGoogleStatusSaysWhatTheOwnerMustDo() throws {
    let decoder = HubJSON.makeDecoder()
    let off = try decoder.decode(GoogleStatus.self, from: Data(#"{"configured":false}"#.utf8))
    let unconnected = try decoder.decode(GoogleStatus.self, from: Data(#"{"configured":true}"#.utf8))
    let expired = try decoder.decode(GoogleStatus.self, from: Data(#"{"configured":true,"connection":{"email":"me@example.com","scopes":[],"connected_at":"2026-09-28T15:00:00Z","needs_reconnect_since":"2026-10-05T15:00:00Z","last_error":"Token has been expired or revoked."}}"#.utf8))
    let connected = try decoder.decode(GoogleStatus.self, from: Data(#"{"configured":true,"connection":{"email":"me@example.com","scopes":["x"],"connected_at":"2026-09-28T15:00:00Z"}}"#.utf8))

    #expect(!off.needsSignIn && off.summary.contains("no Google OAuth client"))
    #expect(unconnected.needsSignIn && unconnected.summary == "Not connected.")
    #expect(expired.needsSignIn && expired.summary == "Google needs you to connect again.")
    #expect(!connected.needsSignIn && connected.summary.hasPrefix("Connected as me@example.com"))
    let older = try decoder.decode(GoogleStatus.self, from: Data(#"{"configured":true,"connection":{"email":"me@example.com","scopes":["x"],"connected_at":"2026-09-28T15:00:00Z"},"missing_scopes":["y"]}"#.utf8))
    #expect(older.needsSignIn && older.summary.contains("Connect again"))
}
