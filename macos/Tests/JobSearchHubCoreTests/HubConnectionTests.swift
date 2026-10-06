import Foundation
@testable import JobSearchHubCore
import Testing

/// Preferences of their own, so a test never reads the hub URL saved on this Mac.
private func makePreferences() -> UserDefaults {
    let name = "HubConnectionTests.\(UUID().uuidString)"
    let preferences = UserDefaults(suiteName: name)!
    preferences.removePersistentDomain(forName: name)
    return preferences
}

@MainActor @Test func inQAModeTheClientUsesHUB_OWNER_TOKENWithNoKeychainEntry() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub", "--qa-mode"], environment: ["HUB_OWNER_TOKEN": "qa-token"],
        preferences: makePreferences(), readKeychain: { nil }
    )
    await connection.finishReadingToken()

    let client = connection.makeClient()
    #expect(client?.token == "qa-token")
    #expect(client?.baseURL == URL(string: HubConnection.defaultHubURL))
}

@MainActor @Test func outsideQAModeHUB_OWNER_TOKENIsIgnoredAndTheKeychainTokenIsUsed() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: ["HUB_OWNER_TOKEN": "env-token"],
        preferences: makePreferences(), readKeychain: { "keychain-token" }
    )
    await connection.finishReadingToken()

    #expect(connection.makeClient()?.token == "keychain-token")
}

@MainActor @Test func outsideQAModeWithNoKeychainEntryThereIsNoClient() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: ["HUB_OWNER_TOKEN": "env-token"],
        preferences: makePreferences(), readKeychain: { nil }
    )
    await connection.finishReadingToken()

    #expect(connection.token == .missing)
    #expect(connection.makeClient() == nil)
}

@MainActor @Test func inQAModeWithoutATokenTheKeychainIsReadAsUsual() async {
    let environments: [[String: String]] = [[:], ["HUB_OWNER_TOKEN": ""]]
    for environment in environments {
        let connection = HubConnection(
            arguments: ["JobSearchHub", "--qa-mode"], environment: environment,
            preferences: makePreferences(), readKeychain: { "keychain-token" }
        )
        await connection.finishReadingToken()

        #expect(connection.makeClient()?.token == "keychain-token")
    }
}

private struct RefusedByKeychain: Error {}

@MainActor @Test func aBuildWithoutATeamKeepsTheTokenInItsPreferencesWhenTheKeychainRefusesIt() async throws {
    let preferences = makePreferences()
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    await connection.finishReadingToken()

    try connection.save(newToken: " saved-token \n")

    #expect(connection.makeClient()?.token == "saved-token")
    #expect(connection.tokenSource == .preferences)

    let relaunched = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { Issue.record("read the Keychain with a token in preferences"); return nil }
    )
    await relaunched.finishReadingToken()
    #expect(relaunched.makeClient()?.token == "saved-token")
    #expect(relaunched.tokenSource == .preferences)
}

@MainActor @Test func aTeamSignedBuildStillFailsTheSaveWhenTheKeychainRefusesIt() async {
    let preferences = makePreferences()
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: true,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    await connection.finishReadingToken()

    #expect(throws: RefusedByKeychain.self) { try connection.save(newToken: "saved-token") }
    #expect(connection.makeClient() == nil)
    #expect(preferences.string(forKey: HubConnection.ownerTokenPreferenceKey) == nil)
}

@MainActor @Test func aTeamSignedBuildIgnoresATokenInPreferences() async {
    let preferences = makePreferences()
    preferences.set("preferences-token", forKey: HubConnection.ownerTokenPreferenceKey)
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: true,
        readKeychain: { "keychain-token" }
    )
    await connection.finishReadingToken()

    #expect(connection.makeClient()?.token == "keychain-token")
    #expect(connection.tokenSource == .keychain)
}

@MainActor @Test func aSaveTheKeychainTakesDropsTheCopyInPreferences() async throws {
    let preferences = makePreferences()
    preferences.set("old-token", forKey: HubConnection.ownerTokenPreferenceKey)
    var keychainToken: String?
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }, saveKeychain: { keychainToken = $0 }
    )
    await connection.finishReadingToken()
    #expect(connection.tokenSource == .preferences)

    try connection.save(newToken: "new-token")

    #expect(keychainToken == "new-token")
    #expect(connection.makeClient()?.token == "new-token")
    #expect(connection.tokenSource == .keychain)
    #expect(preferences.string(forKey: HubConnection.ownerTokenPreferenceKey) == nil)
}
