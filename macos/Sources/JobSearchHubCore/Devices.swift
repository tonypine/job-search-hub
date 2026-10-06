import Foundation

/// A phone paired with the hub.
public struct Device: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var name: String
    public var createdAt: Date
    public var lastSeenAt: Date?
    public var revokedAt: Date?
    /// The version of the app the phone last called the hub with; nil until
    /// it sends one.
    public var appVersion: String?
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

/// The address a phone reaches the hub at, which its pairing link carries.
public enum PhoneHubAddress {
    private static let hostsUnreachableFromPhone: Set<String> = ["10.0.2.2", "localhost", "127.0.0.1", "::1"]

    /// Whether a real phone can't reach the hub at `address`: there's none
    /// yet, or it's the emulator's alias for the Mac or the Mac's own loopback.
    public static func isUnreachableFromPhone(_ address: String) -> Bool {
        guard let host = URLComponents(string: address.trimmingCharacters(in: .whitespaces))?.host?.lowercased() else {
            return true
        }
        return host.isEmpty || hostsUnreachableFromPhone.contains(host)
    }

    /// This Mac's HTTPS address on the tailnet, from `tailscale status --json`.
    /// Nil until the tailnet issues it certificates, since `tailscale serve`
    /// can't publish HTTPS without them.
    public static func parseTailscaleStatusToAddress(_ statusJSON: Data) -> String? {
        guard let status = try? JSONDecoder().decode(TailscaleStatus.self, from: statusJSON),
              let domain = status.certDomains?.first(where: { !$0.isEmpty })
        else { return nil }
        return "https://" + domain
    }

    private struct TailscaleStatus: Decodable {
        let certDomains: [String]?

        enum CodingKeys: String, CodingKey {
            case certDomains = "CertDomains"
        }
    }
}
