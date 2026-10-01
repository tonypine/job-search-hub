import Foundation

/// A CV's content, the part of JSON Resume the app reads and edits.
public struct CVContent: Codable, Equatable, Sendable {
    public var basics: CVBasics
    public var work: [CVWork]
}

public struct CVBasics: Codable, Equatable, Sendable {
    public var name: String
    public var label: String
    public var summary: String
}

public struct CVWork: Codable, Equatable, Sendable {
    public var name: String
    public var position: String
    public var startDate: String
    public var endDate: String?
    public var highlights: [String]
}

/// The base CV, or a draft tailored for a job. A tailored draft's citations
/// map each bullet ("w0h1") to its source: a base bullet ("base:w2h0") or a
/// confirmed entry ("entry:<id>").
public struct CV: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var content: CVContent
    public var citations: [String: String]
    public var hasPDF: Bool

    enum CodingKeys: String, CodingKey {
        case id, kind, content, citations
        case hasPDF = "hasPdf"
    }
}

/// A bullet as an edit sends it: its words and its source.
public struct CVBulletEdit: Codable, Equatable, Sendable {
    public var text: String
    public var source: String

    public init(text: String, source: String) {
        self.text = text
        self.source = source
    }
}

public struct CVRoleEdit: Codable, Equatable, Sendable {
    public var role: Int
    public var bullets: [CVBulletEdit]

    public init(role: Int, bullets: [CVBulletEdit]) {
        self.role = role
        self.bullets = bullets
    }
}

/// The owner's edit of a tailored CV: its headline, summary and every role's
/// bullets, each still citing its source.
public struct CVEdit: Codable, Equatable, Sendable {
    public var label: String
    public var summary: String
    public var roles: [CVRoleEdit]

    public init(label: String, summary: String, roles: [CVRoleEdit]) {
        self.label = label
        self.summary = summary
        self.roles = roles
    }
}

/// How a tailored bullet stands against the base CV.
public enum CVBulletStatus: Equatable, Sendable {
    /// The base bullet, word for word.
    case kept
    /// A base bullet, reworded for the job.
    case reworded(from: String)
    /// A confirmed knowledge-base entry, added for the job.
    case fromEntry
}

public struct CVBulletComparison: Equatable, Sendable {
    public var text: String
    public var source: String
    public var status: CVBulletStatus
}

public struct CVRoleComparison: Equatable, Identifiable, Sendable {
    public var index: Int
    public var position: String
    public var company: String
    public var bullets: [CVBulletComparison]
    /// The base bullets of the role the draft leaves out.
    public var leftOut: [String]

    public var id: Int { index }
}

/// A tailored CV against the base CV: what changed in the headline and
/// summary, and each role's bullets.
public struct CVComparison: Equatable, Sendable {
    public var isLabelChanged: Bool
    public var isSummaryChanged: Bool
    public var roles: [CVRoleComparison]

    public init(tailored: CV, base: CV) {
        isLabelChanged = tailored.content.basics.label != base.content.basics.label
        isSummaryChanged = tailored.content.basics.summary != base.content.basics.summary
        roles = tailored.content.work.enumerated().map { workIndex, work in
            var citedBase = Set<String>()
            let bullets = work.highlights.enumerated().map { highlightIndex, text in
                let source = tailored.citations["w\(workIndex)h\(highlightIndex)"] ?? ""
                if let baseText = Self.getBaseBullet(source, in: base.content) {
                    citedBase.insert(source)
                    return CVBulletComparison(text: text, source: source, status: text == baseText ? .kept : .reworded(from: baseText))
                }
                return CVBulletComparison(text: text, source: source, status: .fromEntry)
            }
            let baseHighlights = workIndex < base.content.work.count ? base.content.work[workIndex].highlights : []
            let leftOut = baseHighlights.enumerated().filter { !citedBase.contains("base:w\(workIndex)h\($0.offset)") }.map(\.element)
            return CVRoleComparison(index: workIndex, position: work.position, company: work.name, bullets: bullets, leftOut: leftOut)
        }
    }

    /// The base bullet a "base:w2h0" source names; nil for any other source.
    static func getBaseBullet(_ source: String, in base: CVContent) -> String? {
        guard source.hasPrefix("base:w"), let hIndex = source.firstIndex(of: "h") else { return nil }
        let workText = source[source.index(source.startIndex, offsetBy: 6)..<hIndex]
        guard let work = Int(workText), let highlight = Int(source[source.index(after: hIndex)...]),
              work < base.work.count, highlight < base.work[work].highlights.count
        else { return nil }
        return base.work[work].highlights[highlight]
    }
}

public extension HubClient {
    func getBaseCV() async throws -> CV {
        try await get("v1/cvs/base", as: CV.self)
    }

    /// The job's tailored CV; HubError.notFound until one is drafted.
    func getJobCV(_ jobID: UUID) async throws -> CV {
        try await get("v1/jobs/\(jobID.uuidString)/cv", as: CV.self)
    }

    func saveCVEdit(_ jobID: UUID, _ edit: CVEdit) async throws -> CV {
        try await send("PUT", "v1/jobs/\(jobID.uuidString)/cv", body: edit, as: CV.self)
    }

    /// Asks Claude for a new draft; it's written in the background.
    func draftCV(_ jobID: UUID) async throws {
        _ = try await send("POST", "v1/jobs/\(jobID.uuidString)/cv", body: EmptyBody(), as: FullBriefResponse.self)
    }

    func getCVHTML(_ cvID: UUID) async throws -> String {
        String(decoding: try await getData("v1/cvs/\(cvID.uuidString)/html"), as: UTF8.self)
    }

    func uploadCVPDF(_ cvID: UUID, _ pdf: Data) async throws -> CV {
        try await upload("v1/cvs/\(cvID.uuidString)/pdf", data: pdf, contentType: "application/pdf", method: "PUT", as: CV.self)
    }
}
