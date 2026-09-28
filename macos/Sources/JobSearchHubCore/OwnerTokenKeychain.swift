import Foundation
import Security

/// The owner token, kept in the login Keychain under this app's own item.
///
/// The app writes the item itself, so the item's access list names the app
/// and a rebuild signed with the same Apple identity keeps reading it without
/// a prompt. Reads never show UI: a read that would need a prompt fails fast
/// instead of hanging the app.
public enum OwnerTokenKeychain {
    static let service = "com.tonypine.JobSearchHub"
    static let account = "owner-token"

    public static func read() -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            // Deprecated, but it is the flag verified to fail fast on the file-based
            // Keychain's access prompt; without it an unanswerable prompt hangs the app.
            kSecUseAuthenticationUI as String: kSecUseAuthenticationUIFail,
        ]
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess, let data = item as? Data else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    public static func save(_ token: String) throws {
        let identity: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        let value = Data(token.utf8)
        let updateStatus = SecItemUpdate(identity as CFDictionary, [kSecValueData as String: value] as CFDictionary)
        if updateStatus == errSecSuccess { return }
        guard updateStatus == errSecItemNotFound else { throw KeychainError(status: updateStatus) }

        var addition = identity
        addition[kSecValueData as String] = value
        let addStatus = SecItemAdd(addition as CFDictionary, nil)
        guard addStatus == errSecSuccess else { throw KeychainError(status: addStatus) }
    }
}

public struct KeychainError: Error, CustomStringConvertible {
    public let status: OSStatus

    public var description: String {
        let message = SecCopyErrorMessageString(status, nil) as String? ?? "unknown error"
        return "Keychain error \(status): \(message)"
    }
}
