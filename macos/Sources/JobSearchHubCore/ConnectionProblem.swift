import Foundation

/// Why the app can't work with the hub, as the window's one connection banner
/// says it; nil when it can.
public enum ConnectionProblem: Equatable, Sendable {
    /// The owner token is still being read; the Keychain may be asking.
    case waitingForKeychain
    /// No hub address or owner token yet.
    case notSetUp
    /// Nothing answers at the hub's address.
    case unreachable
    case tokenRefused
    /// The hub no longer serves this version of the app, in its words.
    case upgradeRequired(String)
    /// The check failed some other way, with the raw reason.
    case failed(String)

    /// The banner's problem from what the app knows: the token, whether it
    /// has a client, the last check, and the event stream, which proves the
    /// hub answers whatever an older check said. A check not made yet shows
    /// no banner, so launching doesn't flash one.
    public static func diagnose(
        isReadingToken: Bool, hasClient: Bool, status: ConnectionStatus, isStreamConnected: Bool
    ) -> ConnectionProblem? {
        if isReadingToken { return .waitingForKeychain }
        guard hasClient else { return .notSetUp }
        if isStreamConnected { return nil }
        switch status {
        case .unchecked, .waitingForKeychain, .connected: return nil
        case .missingToken: return .notSetUp
        case .serverUnreachable: return .unreachable
        case .tokenRefused: return .tokenRefused
        case let .upgradeRequired(message): return .upgradeRequired(message)
        case let .failed(reason): return .failed(reason)
        }
    }

    /// "Can't reach the hub at localhost:8090".
    public func describe(hubAddress: String) -> String {
        switch self {
        case .waitingForKeychain: "Waiting for Keychain access to the owner token"
        case .notSetUp: "Set the hub's address and owner token in Settings"
        case .unreachable: "Can't reach the hub at \(hubAddress)"
        case .tokenRefused: "The hub at \(hubAddress) refused the owner token"
        case let .upgradeRequired(message): message
        case .failed: "Can't connect to the hub at \(hubAddress)"
        }
    }

    /// Starting the server on this Mac can fix it.
    public var isFixedByStartingTheServer: Bool { self == .unreachable }

    /// The address as the banner names it: host and port, without the scheme.
    public static func describeAddress(_ url: URL?, typed: String) -> String {
        guard let url, let host = url.host() else { return typed }
        return url.port.map { "\(host):\($0)" } ?? host
    }
}
