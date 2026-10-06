import Foundation
import JobSearchHubCore
import Observation

/// The app's link to the hub: its URL (in preferences), the owner token (in
/// the Keychain) and the last connection check. Pages build their API client
/// from it.
///
/// The token is read once, off the main thread, and held in memory: when the
/// Keychain item's access list doesn't name this build, the read waits on the
/// Keychain's access prompt, and the window stays responsive meanwhile. A
/// token the Keychain refuses, as in a VM whose login keychain is locked, is
/// still used until the app quits.
@MainActor
@Observable
final class HubConnection {
    static let defaultHubURL = "http://localhost:8090"
    private static let hubURLPreferenceKey = "hubURL"

    var hubURLText: String
    private(set) var token: OwnerTokenState = .reading
    private(set) var status: ConnectionStatus = .unchecked
    private(set) var isChecking = false
    @ObservationIgnored private var tokenRead: Task<Void, Never>?

    /// With an imported token, from `--import-owner-token`, the app uses it
    /// rather than read the Keychain, which may have refused it.
    init(importedToken: OwnerTokenState? = nil) {
        hubURLText = UserDefaults.standard.string(forKey: Self.hubURLPreferenceKey) ?? Self.defaultHubURL
        if let importedToken {
            token = importedToken
            return
        }
        tokenRead = Task {
            let token = await OwnerTokenKeychain.readOffMainThread()
            if self.token == .reading {
                self.token = OwnerTokenState(read: token)
            }
        }
    }

    var hasToken: Bool { token.value != nil }

    var hubURL: URL? {
        guard let url = URL(string: hubURLText.trimmingCharacters(in: .whitespaces)),
              url.scheme == "http" || url.scheme == "https", url.host() != nil
        else { return nil }
        return url
    }

    /// A client for the pages, or nil until a URL and a token are set, and
    /// while the token is still being read.
    func makeClient() -> HubClient? {
        guard let hubURL, let token = token.value else { return nil }
        return HubClient(baseURL: hubURL, token: token)
    }

    /// Saves the URL, and the token when one is given; an empty token field
    /// keeps the stored token. A token the Keychain refuses is unsaved, and
    /// used until the app quits.
    func save(newToken: String) {
        UserDefaults.standard.set(hubURLText, forKey: Self.hubURLPreferenceKey)
        let trimmedToken = newToken.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmedToken.isEmpty {
            token = .saving(trimmedToken)
        }
    }

    func check() async {
        guard let hubURL else {
            status = .failed("the hub URL is not a valid http(s) address")
            return
        }
        isChecking = true
        if token == .reading {
            status = .waitingForKeychain
            await tokenRead?.value
        }
        status = await ConnectionStatus.check(baseURL: hubURL, token: token.value)
        isChecking = false
    }
}
