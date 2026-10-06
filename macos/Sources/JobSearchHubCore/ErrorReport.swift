import Foundation

/// An error as the app shows it: what to do about it in plain words, and the
/// error itself, which folds away under Details.
public struct ErrorReport: Equatable, Sendable {
    public var advice: String
    /// The raw error, for a bug report or the hub's log; nil when the advice
    /// says everything.
    public var details: String?

    public init(advice: String, details: String? = nil) {
        self.advice = advice
        self.details = details
    }

    public init(_ error: any Error) {
        self.init(advice: Self.getAdvice(for: error), details: String(describing: error))
    }

    static func getAdvice(for error: any Error) -> String {
        switch error {
        case let error as HubError:
            switch error {
            case .unreachable:
                "The hub didn't answer. It may be stopped or restarting; try again in a moment."
            case .unauthorized:
                "The hub didn't accept the owner token. Check it in Settings."
            case .forbidden:
                "The hub doesn't let this token do that."
            case .notFound:
                "The hub doesn't have it any more. It may have been removed."
            case let .server(status, message) where (400..<500).contains(status) && !message.isEmpty:
                "The hub turned it down: \(message)"
            case .server:
                "The hub ran into a problem. Try again; if it keeps happening, the hub's log says why."
            case .undecodable:
                "The hub's answer didn't make sense to the app. The app and the hub may be different versions."
            case let .upgradeRequired(message):
                message
            }
        case is CancellationError:
            "It was cancelled."
        default:
            "Something went wrong. Try again; the details below say what."
        }
    }
}
