import Foundation

/// The owner's LinkedIn profile as their export gives it.
public struct LinkedInProfile: Decodable, Equatable, Sendable {
    public var headline: String?
    public var summary: String?
    public var location: String?
    public var positions: [LinkedInPosition]?
    public var skills: [String]?
    public var languages: [LinkedInLanguage]?
    public var updatedAt: Date?

    public var isEmpty: Bool { headline == nil && (positions ?? []).isEmpty && (skills ?? []).isEmpty }
}

public struct LinkedInPosition: Decodable, Equatable, Sendable {
    public var company: String
    public var title: String
    public var startedOn: String?
    public var finishedOn: String?

    /// "Jan 2024 – now".
    public var period: String { "\(startedOn ?? "?") – \((finishedOn?.isEmpty ?? true) ? "now" : finishedOn!)" }
}

public struct LinkedInLanguage: Decodable, Equatable, Sendable {
    public var name: String
    public var proficiency: String?
}

/// The profile, and where what LinkedIn tells recruiters differs from the
/// hub's criteria.
public struct LinkedInProfileResponse: Decodable, Sendable {
    public var profile: LinkedInProfile
    public var criteriaDifferences: [String]
}

public struct ProfileImportRequest: Encodable, Sendable {
    public var files: [String: String]

    public init(files: [String: String]) {
        self.files = files
    }
}

public struct ProfileImportResponse: Decodable, Sendable {
    public var read: [String]

    public var summary: String { "Profile: read \(read.joined(separator: ", "))." }
}

extension LinkedInArchive {
    /// The files that make up the profile, sent together.
    public static let profileFileNames: Set<String> = [
        "Profile.csv", "Profile Summary.csv", "Positions.csv", "Skills.csv", "Education.csv", "Languages.csv", "Projects.csv",
        "Courses.csv", "Job Seeker Preferences.csv",
    ]

    /// The profile files among an export's files.
    public static func findProfileFiles(in files: [URL]) -> [URL] {
        files.filter { profileFileNames.contains($0.lastPathComponent) }
    }
}
