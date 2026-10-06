import Foundation
@testable import JobSearchHubCore
import Security
import Testing

@Test func theOwnerTokenQueryAsksToSkipItemsThatNeedUI() {
    let query = OwnerTokenKeychain.readQuery
    #expect(query[kSecUseAuthenticationUI as String] as? String == kSecUseAuthenticationUISkip as String)
    #expect(query[kSecAttrService as String] as? String == "com.tonypine.JobSearchHub")
    #expect(query[kSecAttrAccount as String] as? String == "owner-token")
    #expect(query[kSecReturnData as String] as? Bool == true)
}

@MainActor @Test func theOwnerTokenIsReadOffTheMainThread() async {
    let token = await OwnerTokenKeychain.readOffMainThread {
        Thread.isMainThread ? "read on the main thread" : "owner-token"
    }
    #expect(token == "owner-token")
}

@Test func aReadTokenIsPresentAndNoTokenIsMissing() {
    #expect(OwnerTokenState(read: "owner-token") == .present("owner-token"))
    #expect(OwnerTokenState(read: "owner-token").value == "owner-token")
    #expect(OwnerTokenState(read: nil) == .missing)
    #expect(OwnerTokenState(read: "") == .missing)
    #expect(OwnerTokenState.reading.value == nil)
}

@Test func aSavedTokenIsPresent() {
    var saved: String?
    let state = OwnerTokenState(saving: "owner-token") { saved = $0 }
    #expect(saved == "owner-token")
    #expect(state == .present("owner-token"))
}

@Test func aTokenTheKeychainRefusesIsStillUsed() {
    let state = OwnerTokenState(saving: "owner-token") { _ in throw KeychainError(status: errSecNoSuchKeychain) }
    #expect(state.value == "owner-token")
    guard case .unsaved(_, let reason) = state else {
        Issue.record("expected an unsaved token, got \(state)")
        return
    }
    #expect(reason.hasPrefix("Keychain error \(errSecNoSuchKeychain):"))
}

@Test func waitingOnTheKeychainIsSaidSo() {
    #expect(ConnectionStatus.waitingForKeychain.message == "Waiting for Keychain access.")
}
