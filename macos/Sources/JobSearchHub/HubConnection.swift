import Foundation
import JobSearchHubCore
import Observation

/// The app's link to the hub: its URL (in preferences), the owner token (in
/// the Keychain) and the last connection check. Pages build their API client
/// from it.
@MainActor
@Observable
final class HubConnection {
    static let defaultHubURL = "http://localhost:8090"
    private static let hubURLPreferenceKey = "hubURL"

    var hubURLText: String
    private(set) var hasToken: Bool
    private(set) var status: ConnectionStatus = .unchecked
    private(set) var isChecking = false

    init() {
        hubURLText = UserDefaults.standard.string(forKey: Self.hubURLPreferenceKey) ?? Self.defaultHubURL
        hasToken = OwnerTokenKeychain.read() != nil
    }

    var hubURL: URL? {
        guard let url = URL(string: hubURLText.trimmingCharacters(in: .whitespaces)),
              url.scheme == "http" || url.scheme == "https", url.host() != nil
        else { return nil }
        return url
    }

    /// A client for the pages, or nil until a URL and a token are set.
    func makeClient() -> HubClient? {
        guard let hubURL, let token = OwnerTokenKeychain.read() else { return nil }
        return HubClient(baseURL: hubURL, token: token)
    }

    /// Saves the URL, and the token when one is given; an empty token field
    /// keeps the stored token.
    func save(newToken: String) throws {
        UserDefaults.standard.set(hubURLText, forKey: Self.hubURLPreferenceKey)
        let trimmedToken = newToken.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmedToken.isEmpty {
            try OwnerTokenKeychain.save(trimmedToken)
        }
        hasToken = OwnerTokenKeychain.read() != nil
    }

    func check() async {
        guard let hubURL else {
            status = .failed("the hub URL is not a valid http(s) address")
            return
        }
        isChecking = true
        status = await ConnectionStatus.check(baseURL: hubURL, token: OwnerTokenKeychain.read())
        isChecking = false
    }
}
