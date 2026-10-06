import Foundation

/// Where the app stands with the hub, as Settings › Connection reports it.
public enum ConnectionStatus: Equatable, Sendable {
    case unchecked
    case waitingForKeychain
    case missingToken
    case serverUnreachable(String)
    case tokenRefused
    /// The hub no longer serves this version of the app, in its words.
    case upgradeRequired(String)
    case connected
    case failed(String)

    public var message: String {
        switch self {
        case .unchecked: "Not checked yet."
        case .waitingForKeychain: "Waiting for Keychain access."
        case .missingToken: "No owner token saved."
        case .serverUnreachable(let reason): "The hub is not reachable: \(reason)"
        case .tokenRefused: "The hub refused the token."
        case .upgradeRequired(let message): message
        case .connected: "Connected."
        case .failed(let reason): "The check failed: \(reason)"
        }
    }

    /// The status in a word or two, for its status row; `message` says why.
    public var title: String {
        switch self {
        case .unchecked: "Not checked"
        case .waitingForKeychain: "Waiting for Keychain"
        case .missingToken: "No token"
        case .serverUnreachable: "Not reachable"
        case .tokenRefused: "Token refused"
        case .upgradeRequired: "Update the app"
        case .connected: "Connected"
        case .failed: "Check failed"
        }
    }

    /// Checks the server's health first, so a stopped server is not reported
    /// as a bad token, then makes one authenticated call.
    public static func check(baseURL: URL, token: String?, session: URLSession = .shared) async -> ConnectionStatus {
        do {
            let (_, response) = try await session.data(from: baseURL.appending(path: "v1/health"))
            if (response as? HTTPURLResponse)?.statusCode != 200 {
                return .serverUnreachable("its health check did not answer OK")
            }
        } catch {
            return .serverUnreachable(error.localizedDescription)
        }

        guard let token, !token.isEmpty else { return .missingToken }
        do {
            _ = try await HubClient(baseURL: baseURL, token: token, session: session).get("v1/profile", as: OwnerProfile.self)
            return .connected
        } catch HubError.unauthorized, HubError.forbidden {
            return .tokenRefused
        } catch HubError.upgradeRequired(let message) {
            return .upgradeRequired(message)
        } catch {
            return .failed(String(describing: error))
        }
    }
}

public struct OwnerProfile: Codable, Equatable, Sendable {
    public var body: String
    public var updatedAt: Date
}
