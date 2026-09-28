import Foundation

public struct GoogleStatus: Decodable, Equatable, Sendable {
    /// False when the hub has no Google OAuth client file.
    public var configured: Bool
    public var connection: GoogleConnection?

    /// What the Settings section says about the connection.
    public var summary: String {
        guard configured else { return "The hub has no Google OAuth client file." }
        guard let connection else { return "Not connected." }
        if connection.needsReconnectSince != nil { return "Google needs you to connect again." }
        return "Connected as \(connection.email) since \(connection.connectedAt.formatted(date: .abbreviated, time: .shortened))."
    }

    public var needsSignIn: Bool {
        configured && (connection == nil || connection?.needsReconnectSince != nil)
    }
}

public struct GoogleConnection: Decodable, Equatable, Sendable {
    public var email: String
    public var scopes: [String]
    public var connectedAt: Date
    public var needsReconnectSince: Date?
    public var lastError: String?
}

public struct GoogleSignIn: Decodable, Sendable {
    public var url: String
}

public struct GoogleCheck: Decodable, Equatable, Sendable {
    public var email: String
    public var labelCount: Int
    public var calendarCount: Int
}
