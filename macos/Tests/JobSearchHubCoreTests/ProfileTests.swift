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
