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
    let preferences = makePreferences()
    preferences.set("preferences-token", forKey: HubConnection.ownerTokenPreferenceKey)
    let connection = HubConnection(
        arguments: ["JobSearchHub", "--qa-mode"], environment: ["HUB_OWNER_TOKEN": "qa-token"],
        preferences: preferences, isTeamSigned: false, readKeychain: { nil }
    )
    await connection.finishReadingToken()

    let client = connection.makeClient()
    #expect(client?.token == "qa-token")
    #expect(client?.baseURL == URL(string: HubConnection.defaultHubURL))
    #expect(connection.tokenSource == .environment)
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

@MainActor @Test func inQAModeWithoutATokenTheKeychainIsNotRead() async {
    let environments: [[String: String]] = [[:], ["HUB_OWNER_TOKEN": ""]]
    for environment in environments {
        let connection = HubConnection(
            arguments: ["JobSearchHub", "--qa-mode"], environment: environment, preferences: makePreferences(),
            readKeychain: { Issue.record("read the Keychain in QA mode"); return "keychain-token" }
        )
        await connection.finishReadingToken()

        #expect(connection.token == .missing)
        #expect(connection.makeClient() == nil)
    }
}

@MainActor @Test func inQAModeTheHubURLAndTokenAnEarlierRunSavedAreForgotten() async {
    let preferences = makePreferences()
    let earlierRun = HubConnection(
        arguments: ["JobSearchHub", "--qa-mode"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    earlierRun.hubURLText = "http://localhost:65442"
    earlierRun.save(newToken: "earlier-token")
    #expect(earlierRun.makeClient()?.token == "earlier-token")

    let connection = HubConnection(
        arguments: ["JobSearchHub", "--qa-mode"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }
    )
    await connection.finishReadingToken()

    #expect(connection.hubURLText == HubConnection.defaultHubURL)
    #expect(connection.token == .missing)
    #expect(connection.makeClient() == nil)
    #expect(preferences.string(forKey: HubConnection.ownerTokenPreferenceKey) == nil)
}

private struct RefusedByKeychain: Error {}

@MainActor @Test func aQABuildStartsFromEmptyConnectionSettingsWithoutTheFlag() async {
    let preferences = makePreferences()
    let earlierRun = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false, isQABuild: true,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    earlierRun.hubURLText = "http://localhost:65442"
    earlierRun.save(newToken: "earlier-token")

    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false, isQABuild: true,
        readKeychain: { Issue.record("read the Keychain in a QA build"); return "keychain-token" }
    )
    await connection.finishReadingToken()

    #expect(connection.hubURLText == HubConnection.defaultHubURL)
    #expect(connection.token == .missing)
    #expect(connection.makeClient() == nil)
}

@MainActor @Test func aQABuildUsesHUB_OWNER_TOKENWithoutTheFlag() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: ["HUB_OWNER_TOKEN": "qa-token"],
        preferences: makePreferences(), isTeamSigned: false, isQABuild: true, readKeychain: { nil }
    )

    #expect(connection.makeClient()?.token == "qa-token")
    #expect(connection.tokenSource == .environment)
}

@MainActor @Test func aBuildWithoutATeamKeepsTheTokenInItsPreferencesWhenTheKeychainRefusesIt() async {
    let preferences = makePreferences()
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    await connection.finishReadingToken()

    connection.save(newToken: " saved-token \n")

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

@MainActor @Test func aTeamSignedBuildLeavesATokenTheKeychainRefusesUnsaved() async {
    let preferences = makePreferences()
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: true,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    await connection.finishReadingToken()

    connection.save(newToken: "saved-token")

    guard case .unsaved("saved-token", _) = connection.token else {
        Issue.record("expected an unsaved token, got \(connection.token)")
        return
    }
    #expect(connection.makeClient()?.token == "saved-token")
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

@MainActor @Test func aSaveTheKeychainTakesDropsTheCopyInPreferences() async {
    let preferences = makePreferences()
    preferences.set("old-token", forKey: HubConnection.ownerTokenPreferenceKey)
    var keychainToken: String?
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:], preferences: preferences, isTeamSigned: false,
        readKeychain: { nil }, saveKeychain: { keychainToken = $0 }
    )
    await connection.finishReadingToken()
    #expect(connection.tokenSource == .preferences)

    connection.save(newToken: "new-token")

    #expect(keychainToken == "new-token")
    #expect(connection.makeClient()?.token == "new-token")
    #expect(connection.tokenSource == .keychain)
    #expect(preferences.string(forKey: HubConnection.ownerTokenPreferenceKey) == nil)
}

@MainActor @Test func anImportedTokenTheKeychainRefusedIsUsedWithoutReadingTheKeychain() async {
    let connection = HubConnection(
        importedToken: .unsaved("imported-token", reason: "locked"),
        arguments: ["JobSearchHub"], environment: [:],
        preferences: makePreferences(), isTeamSigned: true, readKeychain: { "keychain-token" }
    )
    await connection.finishReadingToken()

    #expect(connection.makeClient()?.token == "imported-token")
}

@MainActor @Test func aTypedTokenTheKeychainRefusesStillMakesAClient() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:],
        preferences: makePreferences(), isTeamSigned: true,
        readKeychain: { nil }, saveKeychain: { _ in throw RefusedByKeychain() }
    )
    await connection.finishReadingToken()

    connection.save(newToken: " typed-token \n")

    #expect(connection.token == .unsaved("typed-token", reason: String(describing: RefusedByKeychain())))
    #expect(connection.makeClient()?.token == "typed-token")
}

@MainActor @Test func aBuildWithoutATeamKeepsAnImportedTokenTheKeychainRefusedInItsPreferences() async {
    let preferences = makePreferences()
    let connection = HubConnection(
        importedToken: .unsaved("imported-token", reason: "locked"),
        arguments: ["JobSearchHub"], environment: [:],
        preferences: preferences, isTeamSigned: false, readKeychain: { nil }
    )
    await connection.finishReadingToken()

    #expect(connection.token == .present("imported-token"))
    #expect(connection.tokenSource == .preferences)
    #expect(preferences.string(forKey: HubConnection.ownerTokenPreferenceKey) == "imported-token")
}
