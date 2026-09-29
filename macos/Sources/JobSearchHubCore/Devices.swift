import Foundation

/// A phone paired with the hub.
public struct Device: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var name: String
    public var createdAt: Date
    public var lastSeenAt: Date?
    public var revokedAt: Date?
}

public struct DevicesResponse: Decodable, Sendable {
    public var devices: [Device]
}

public struct PairDeviceRequest: Encodable, Sendable {
    public var name: String

    public init(name: String) {
        self.name = name
    }
}

/// A new device and its token, which the hub shows this once.
public struct PairDeviceResponse: Decodable, Sendable {
    public var device: Device
    public var token: String
}

/// The link a pairing QR code carries: where the phone reaches the hub and
/// its token, as `jobsearchhub://pair?url=…&token=…`.
public enum PairingLink {
    public static func make(hubURL: String, token: String) -> String? {
        var components = URLComponents()
        components.scheme = "jobsearchhub"
        components.host = "pair"
        components.queryItems = [URLQueryItem(name: "url", value: hubURL), URLQueryItem(name: "token", value: token)]
        return components.string
    }

    public static func parse(_ link: String) -> (hubURL: String, token: String)? {
        guard let components = URLComponents(string: link), components.scheme == "jobsearchhub", components.host == "pair",
              let hubURL = components.queryItems?.first(where: { $0.name == "url" })?.value, !hubURL.isEmpty,
              let token = components.queryItems?.first(where: { $0.name == "token" })?.value, !token.isEmpty
        else { return nil }
        return (hubURL, token)
    }
}
