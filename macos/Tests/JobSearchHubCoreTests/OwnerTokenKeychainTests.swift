@testable import JobSearchHubCore
import Security
import Testing

@Test func readingTheOwnerTokenSkipsAnItemThatWouldPrompt() {
    let query = OwnerTokenKeychain.readQuery
    #expect(query[kSecUseAuthenticationUI as String] as? String == kSecUseAuthenticationUISkip as String)
    #expect(query[kSecAttrService as String] as? String == "com.tonypine.JobSearchHub")
    #expect(query[kSecAttrAccount as String] as? String == "owner-token")
    #expect(query[kSecReturnData as String] as? Bool == true)
}
