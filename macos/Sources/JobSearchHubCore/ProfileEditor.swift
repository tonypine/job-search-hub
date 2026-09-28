import Foundation
import Observation

/// The Profile page's state. A save shows as saving until the hub answers,
/// and the page then shows what the hub stored; a failed save keeps the edit.
@MainActor
@Observable
public final class ProfileEditor {
    public private(set) var profile: OwnerProfile?
    public var draft = ""
    public private(set) var isEditing = false
    public private(set) var isSaving = false
    public private(set) var errorMessage: String?

    public init() {}

    public func load(with client: HubClient) async {
        do {
            profile = try await client.get("v1/profile", as: OwnerProfile.self)
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }

    public func startEditing() {
        draft = profile?.body ?? ""
        isEditing = true
        errorMessage = nil
    }

    public func cancelEditing() {
        isEditing = false
        errorMessage = nil
    }

    public func save(with client: HubClient) async {
        isSaving = true
        defer { isSaving = false }
        do {
            profile = try await client.send("PUT", "v1/profile", body: ProfileSaveRequest(body: draft), as: OwnerProfile.self)
            isEditing = false
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

struct ProfileSaveRequest: Encodable {
    let body: String
}
