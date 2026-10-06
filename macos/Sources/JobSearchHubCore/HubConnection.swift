import Foundation
import Observation

/// The app's link to the hub: its URL (in preferences), the owner token (in
/// the Keychain) and the last connection check. Pages build their API client
/// from it.
///
/// The token is read once, off the main thread, and held in memory: when the
/// Keychain item's access list doesn't name this build, the read waits on the
/// Keychain's access prompt, and the window stays responsive meanwhile.
///
/// Launched with `--qa-mode`, the app takes the token from HUB_OWNER_TOKEN
/// instead, for a QA machine whose Keychain won't keep it. Without the flag
/// the variable is ignored.
///
/// A build no Apple team signed, such as the ad hoc build in Symphony's QA
/// VM, keeps the token in its preferences when the Keychain refuses it, and
/// reads it from there first. A team-signed build only ever uses the Keychain.
@MainActor
@Observable
public final class HubConnection {
    public static let defaultHubURL = "http://localhost:8090"
    public static let qaModeArgument = "--qa-mode"
    private static let hubURLPreferenceKey = "hubURL"
    static let ownerTokenPreferenceKey = "ownerToken"

    public var hubURLText: String
    public private(set) var token: OwnerTokenState = .reading
    /// Where the token came from, or went on the last save.
    public private(set) var tokenSource: OwnerTokenSource = .keychain
    public private(set) var status: ConnectionStatus = .unchecked
    public private(set) var isChecking = false
    @ObservationIgnored private let preferences: UserDefaults
    @ObservationIgnored private let saveKeychain: (String) throws -> Void
    @ObservationIgnored private let isTeamSigned: Bool
    @ObservationIgnored private var tokenRead: Task<Void, Never>?

    public init(
        arguments: [String] = ProcessInfo.processInfo.arguments,
        environment: [String: String] = ProcessInfo.processInfo.environment,
        preferences: UserDefaults = .standard,
        isTeamSigned: Bool = BuildSignature.hasTeam,
        readKeychain: @escaping @Sendable () async -> String? = { await OwnerTokenKeychain.readOffMainThread() },
        saveKeychain: @escaping (String) throws -> Void = { try OwnerTokenKeychain.save($0) }
    ) {
        self.preferences = preferences
        self.saveKeychain = saveKeychain
        self.isTeamSigned = isTeamSigned
        hubURLText = preferences.string(forKey: Self.hubURLPreferenceKey) ?? Self.defaultHubURL
        if let qaToken = Self.qaModeToken(arguments: arguments, environment: environment) {
            token = .present(qaToken)
            tokenSource = .environment
            return
        }
        if !isTeamSigned, let saved = OwnerTokenState(read: preferences.string(forKey: Self.ownerTokenPreferenceKey)).value {
            token = .present(saved)
            tokenSource = .preferences
            return
        }
        tokenRead = Task {
            let token = await readKeychain()
            if self.token == .reading {
                self.token = OwnerTokenState(read: token)
            }
        }
    }

    /// HUB_OWNER_TOKEN when the app runs in QA mode and the variable holds one.
    static func qaModeToken(arguments: [String], environment: [String: String]) -> String? {
        guard arguments.contains(qaModeArgument) else { return nil }
        return OwnerTokenState(read: environment["HUB_OWNER_TOKEN"]).value
    }

    public var hasToken: Bool { token.value != nil }

    public var hubURL: URL? {
        guard let url = URL(string: hubURLText.trimmingCharacters(in: .whitespaces)),
              url.scheme == "http" || url.scheme == "https", url.host() != nil
        else { return nil }
        return url
    }

    /// A client for the pages, or nil until a URL and a token are set, and
    /// while the token is still being read.
    public func makeClient() -> HubClient? {
        guard let hubURL, let token = token.value else { return nil }
        return HubClient(baseURL: hubURL, token: token)
    }

    /// Saves the URL, and the token when one is given; an empty token field
    /// keeps the stored token. A build without a team keeps the token in its
    /// preferences when the Keychain refuses it, and drops that copy once the
    /// Keychain takes one.
    public func save(newToken: String) throws {
        preferences.set(hubURLText, forKey: Self.hubURLPreferenceKey)
        let trimmedToken = newToken.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedToken.isEmpty else { return }
        do {
            try saveKeychain(trimmedToken)
            preferences.removeObject(forKey: Self.ownerTokenPreferenceKey)
            tokenSource = .keychain
        } catch {
            guard !isTeamSigned else { throw error }
            preferences.set(trimmedToken, forKey: Self.ownerTokenPreferenceKey)
            tokenSource = .preferences
        }
        token = .present(trimmedToken)
    }

    public func check() async {
        guard let hubURL else {
            status = .failed("the hub URL is not a valid http(s) address")
            return
        }
        isChecking = true
        if token == .reading {
            status = .waitingForKeychain
            await finishReadingToken()
        }
        status = await ConnectionStatus.check(baseURL: hubURL, token: token.value)
        isChecking = false
    }

    /// Returns once the Keychain read started at launch is done.
    func finishReadingToken() async {
        await tokenRead?.value
    }
}

/// Where the app holds the owner token.
public enum OwnerTokenSource: Equatable, Sendable {
    case keychain
    /// This build's preferences, for a build without a team whose Keychain refused it.
    case preferences
    /// HUB_OWNER_TOKEN, in QA mode.
    case environment
}
