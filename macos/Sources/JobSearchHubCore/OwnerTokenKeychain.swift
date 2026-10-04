import Foundation
import Security

/// The owner token, kept in the login Keychain under this app's own item.
///
/// The app writes the item itself, so the item's access list names the app
/// and a rebuild signed with the same Apple identity keeps reading it without
/// a prompt. Reads never show UI: an item that would need a prompt is skipped,
/// so the read finds nothing instead of hanging the app.
public enum OwnerTokenKeychain {
    static let service = "com.tonypine.JobSearchHub"
    static let account = "owner-token"

    static var readQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            // The file-based Keychain honors this key, not LAContext.interactionNotAllowed,
            // which only covers Data Protection keychain items; without it an unanswerable
            // access prompt hangs the app. Skip, unlike the deprecated Fail, finds nothing.
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
