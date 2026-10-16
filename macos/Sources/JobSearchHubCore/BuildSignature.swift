import Foundation
import Security

/// Who signed the running app.
public enum BuildSignature {
    /// Whether an Apple team signed this process. A build signed ad hoc, as
    /// make-app.sh signs one without an Apple identity (CI, Symphony's QA VM),
    /// or self-signed, or not signed at all, has no team.
    public static let hasTeam: Bool = {
        var code: SecCode?
        var staticCode: SecStaticCode?
        var information: CFDictionary?
        guard SecCodeCopySelf([], &code) == errSecSuccess, let code,
              SecCodeCopyStaticCode(code, [], &staticCode) == errSecSuccess, let staticCode,
              SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &information) == errSecSuccess,
              let information = information as? [String: Any]
        else { return false }
        return (information[kSecCodeInfoTeamIdentifier as String] as? String)?.isEmpty == false
    }()
}
