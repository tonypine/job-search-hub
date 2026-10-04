import Foundation
import Security

/// The owner token, kept in the login Keychain under this app's own item.
///
/// The app writes the item itself, so the item's access list names the app
/// and a rebuild signed with the same Apple identity keeps reading it without
/// a prompt. A build signed with another identity gets the Keychain's access
/// prompt: on the file-based keychain neither the Fail nor the Skip UI flag
/// suppresses it (tested 2026-10-04).
public enum OwnerTokenKeychain {
    static let service = "com.tonypine.JobSearchHub"
    static let account = "owner-token"

    static var readQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            // Skip rather than the deprecated Fail. Neither suppresses the file-based
            // keychain's access prompt (tested 2026-10-04), so this only drops the
            // deprecation warning.
            kSecUseAuthenticationUI as String: kSecUseAuthenticationUISkip,
        ]
    }

    public static func read() -> String? {
        var item: CFTypeRef?
        guard SecItemCopyMatching(readQuery as CFDictionary, &item) == errSecSuccess, let data = item as? Data else {
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
