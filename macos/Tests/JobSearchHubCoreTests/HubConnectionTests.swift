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

@MainActor @Test func anImportedTokenTheKeychainRefusedIsUsedWithoutReadingTheKeychain() async {
    let connection = HubConnection(
        importedToken: .unsaved("imported-token", reason: "locked"),
        arguments: ["JobSearchHub"], environment: [:],
        preferences: makePreferences(), readKeychain: { "keychain-token" }
    )
    await connection.finishReadingToken()

    #expect(connection.makeClient()?.token == "imported-token")
}

private struct KeychainRefusal: Error {}

@MainActor @Test func aTypedTokenTheKeychainRefusesStillMakesAClient() async {
    let connection = HubConnection(
        arguments: ["JobSearchHub"], environment: [:],
        preferences: makePreferences(), readKeychain: { nil }, saveToKeychain: { _ in throw KeychainRefusal() }
    )
    await connection.finishReadingToken()

    connection.save(newToken: " typed-token \n")

    #expect(connection.token == .unsaved("typed-token", reason: String(describing: KeychainRefusal())))
    #expect(connection.makeClient()?.token == "typed-token")
}
