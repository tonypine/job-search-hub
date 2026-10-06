import Foundation
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
///
/// Launched with `--qa-mode`, the app starts from empty connection settings:
/// it forgets the hub URL and token an earlier run saved, takes the token
/// from HUB_OWNER_TOKEN, and leaves the Keychain alone, for a QA machine whose
/// Keychain won't keep it. Without the flag the variable is ignored. A QA
/// build, whose Info.plist make-app.sh marks with HubQABuild, as it does in
/// Symphony's QA VM, runs in QA mode at every launch, with or without the flag.
///
/// A build no Apple team signed, such as the ad hoc build in Symphony's QA
/// VM, keeps the token in its preferences when the Keychain refuses it, and
/// reads it from there first. A team-signed build never reads or writes that
/// copy.
@MainActor
@Observable
public final class HubConnection {
    public static let defaultHubURL = "http://localhost:8090"
    public static let qaModeArgument = "--qa-mode"
    /// The Info.plist key make-app.sh sets on a QA build.
    public nonisolated static let qaBuildInfoKey = "HubQABuild"
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

    /// With an imported token, from `--import-owner-token`, the app uses it
    /// rather than read the Keychain, which may have refused it.
    public init(
        importedToken: OwnerTokenState? = nil,
        arguments: [String] = ProcessInfo.processInfo.arguments,
        environment: [String: String] = ProcessInfo.processInfo.environment,
        preferences: UserDefaults = .standard,
        isTeamSigned: Bool = BuildSignature.hasTeam,
        isQABuild: Bool = Bundle.main.object(forInfoDictionaryKey: HubConnection.qaBuildInfoKey) as? Bool == true,
        readKeychain: @escaping @Sendable () async -> String? = { await OwnerTokenKeychain.readOffMainThread() },
        saveKeychain: @escaping (String) throws -> Void = { try OwnerTokenKeychain.save($0) }
    ) {
        self.preferences = preferences
        self.saveKeychain = saveKeychain
        self.isTeamSigned = isTeamSigned
        let isQAMode = isQABuild || arguments.contains(Self.qaModeArgument)
        if isQAMode {
            preferences.removeObject(forKey: Self.hubURLPreferenceKey)
            preferences.removeObject(forKey: Self.ownerTokenPreferenceKey)
        }
        hubURLText = preferences.string(forKey: Self.hubURLPreferenceKey) ?? Self.defaultHubURL
        if let importedToken {
            token = keep(importedToken)
            return
        }
        if isQAMode {
            if let qaToken = OwnerTokenState(read: environment["HUB_OWNER_TOKEN"]).value {
                token = .present(qaToken)
                tokenSource = .environment
            } else {
                token = .missing
            }
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
    /// keeps the stored token. A token the Keychain refuses is unsaved, and
    /// used until the app quits, unless the build has no team: it keeps the
    /// token in its preferences instead, and drops that copy once the
    /// Keychain takes one.
    public func save(newToken: String) {
        preferences.set(hubURLText, forKey: Self.hubURLPreferenceKey)
        let trimmedToken = newToken.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedToken.isEmpty else { return }
        token = keep(.saving(trimmedToken, with: saveKeychain))
    }

    /// The state of a token just handed to the Keychain, after a build
    /// without a team has moved one the Keychain refused to its preferences.
    private func keep(_ state: OwnerTokenState) -> OwnerTokenState {
        switch state {
        case .present:
            preferences.removeObject(forKey: Self.ownerTokenPreferenceKey)
            tokenSource = .keychain
            return state
        case .unsaved(let token, _) where !isTeamSigned:
            preferences.set(token, forKey: Self.ownerTokenPreferenceKey)
            tokenSource = .preferences
            return .present(token)
        case .unsaved, .reading, .missing:
            return state
        }
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
