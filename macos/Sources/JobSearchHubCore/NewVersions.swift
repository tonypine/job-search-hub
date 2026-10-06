import Foundation

/// The hub's version, as scripts/release/version.sh stamps it: `0.1.252` for
/// a release, where 252 counts the commits on main, or `0.1.0-dev.abc1234`
/// for a build from a checkout. A lower number is always an older commit, and
/// a local build sorts below the release of the same number.
public struct HubVersion: Comparable, Hashable, Sendable, CustomStringConvertible {
    public let major: Int
    public let minor: Int
    public let patch: Int
    /// What follows the `-`: `dev.abc1234`, or nil for a release.
    public let suffix: String?

    public init?(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let parts = trimmed.split(separator: "-", maxSplits: 1)
        guard let numbers = parts.first else { return nil }
        let fields = numbers.split(separator: ".", omittingEmptySubsequences: false).map { Int($0) }
        guard fields.count == 3, let major = fields[0], let minor = fields[1], let patch = fields[2],
              major >= 0, minor >= 0, patch >= 0
        else { return nil }
        self.major = major
        self.minor = minor
        self.patch = patch
        suffix = parts.count == 2 ? String(parts[1]) : nil
    }

    /// A build from a checkout, which no update installs over by itself.
    public var isLocalBuild: Bool { suffix != nil }

    /// The commit a local build was made from: `abc1234` of
    /// `0.1.0-dev.abc1234`.
    public var localCommit: String? {
        guard let suffix, suffix.hasPrefix("dev.") else { return nil }
        let commit = suffix.dropFirst(4)
        return commit.isEmpty ? nil : String(commit)
    }

    public var description: String {
        "\(major).\(minor).\(patch)" + (suffix.map { "-\($0)" } ?? "")
    }

    public static func < (left: HubVersion, right: HubVersion) -> Bool {
        if (left.major, left.minor, left.patch) != (right.major, right.minor, right.patch) {
            return (left.major, left.minor, left.patch) < (right.major, right.minor, right.patch)
        }
        switch (left.suffix, right.suffix) {
        case (nil, nil), (nil, _?): return false
        case (_?, nil): return true
        case let (leftSuffix?, rightSuffix?): return leftSuffix < rightSuffix
        }
    }
}

/// Where the hub's code and releases live on GitHub. The repository is
/// public, so reading its releases needs no token.
public enum GitHubRepository {
    public static let slug = "tonypine/job-search-hub"

    /// The releases, newest first, as the public API lists them.
    public static let releasesAPIURL = URL(string: "https://api.github.com/repos/\(slug)/releases?per_page=100")!

    /// The releases page, for *Release notes on GitHub*.
    public static let releasesPageURL = URL(string: "https://github.com/\(slug)/releases")!

    public static func makePullRequestURL(_ number: Int) -> URL {
        URL(string: "https://github.com/\(slug)/pull/\(number)")!
    }
}

/// A release as GitHub's API lists it.
public struct GitHubRelease: Decodable, Equatable, Sendable {
    public var tagName: String
    public var name: String?
    public var draft: Bool
    /// A pre-release is a withdrawn one: the apps stop offering it.
    public var prerelease: Bool
    /// The release's notes, in scripts/release/changelog.sh's format.
    public var body: String?
    public var htmlURL: URL?
    public var publishedAt: Date?
    public var assets: [Asset]

    public struct Asset: Decodable, Equatable, Sendable {
        public var name: String
        public var browserDownloadURL: URL
        public var size: Int?

        public init(name: String, browserDownloadURL: URL, size: Int? = nil) {
            self.name = name
            self.browserDownloadURL = browserDownloadURL
            self.size = size
        }

        // HubJSON's decoder turns snake_case keys into camelCase first.
        enum CodingKeys: String, CodingKey {
            case name
            case browserDownloadURL = "browserDownloadUrl"
            case size
        }
    }

    public init(
        tagName: String, name: String? = nil, draft: Bool = false, prerelease: Bool = false, body: String? = nil,
        htmlURL: URL? = nil, publishedAt: Date? = nil, assets: [Asset] = []
    ) {
        self.tagName = tagName
        self.name = name
        self.draft = draft
        self.prerelease = prerelease
        self.body = body
        self.htmlURL = htmlURL
        self.publishedAt = publishedAt
        self.assets = assets
    }

    enum CodingKeys: String, CodingKey {
        case tagName, name, draft, prerelease, body, publishedAt, assets
        case htmlURL = "htmlUrl"
    }

    /// Reads the API's answer: a list of releases.
    public static func decodeList(_ data: Data) throws -> [GitHubRelease] {
        try HubJSON.makeDecoder().decode([GitHubRelease].self, from: data)
    }
}

/// A release of the Mac app: `mac-v0.1.252`, with the zipped app and its
/// checksum.
public struct MacRelease: Equatable, Sendable {
    public var version: HubVersion
    public var release: GitHubRelease

    /// `Job-Search-Hub-0.1.252.zip`, the app as `ditto` zipped it.
    public var zipAsset: GitHubRelease.Asset? {
        release.assets.first { $0.name.hasSuffix(".zip") }
    }

    /// `Job-Search-Hub-0.1.252.zip.sha256`, as `shasum -a 256` wrote it.
    public var checksumAsset: GitHubRelease.Asset? {
        release.assets.first { $0.name.hasSuffix(".zip.sha256") }
    }

    /// Its notes, read from its changelog.
    public var notes: [ChangeNote] {
        ReleaseNotes.parse(release.body ?? "")
    }
}

/// Picks the Mac app's releases out of everything the repository releases.
public enum MacReleases {
    public static let tagPrefix = "mac-v"

    /// Every Mac release with a version in its tag, withdrawn ones included,
    /// newest first. Drafts never count.
    public static func parse(_ releases: [GitHubRelease]) -> [MacRelease] {
        releases.compactMap { release -> MacRelease? in
            guard !release.draft, release.tagName.hasPrefix(tagPrefix),
                  let version = HubVersion(String(release.tagName.dropFirst(tagPrefix.count))), !version.isLocalBuild
            else { return nil }
            return MacRelease(version: version, release: release)
        }
        .sorted { $0.version > $1.version }
    }

    /// The releases the app may offer, newest first: published, and not
    /// withdrawn.
    public static func getOffered(_ releases: [GitHubRelease]) -> [MacRelease] {
        parse(releases).filter { !$0.release.prerelease }
    }

    /// The newest release the app may offer that's newer than the one
    /// running, or nil when there's none.
    public static func findNewest(in releases: [GitHubRelease], above running: HubVersion) -> MacRelease? {
        getOffered(releases).first { $0.version > running }
    }

    /// The releases after the running one, up to and including `newest`,
    /// newest first: the ones *What's new* reads.
    public static func findBetween(in releases: [GitHubRelease], running: HubVersion, through newest: HubVersion) -> [MacRelease] {
        getOffered(releases).filter { $0.version > running && $0.version <= newest }
    }

    /// Whether the running version's release was withdrawn: it's now a
    /// pre-release. A local build has no release, so it never is.
    public static func isWithdrawn(_ running: HubVersion, in releases: [GitHubRelease]) -> Bool {
        guard !running.isLocalBuild else { return false }
        return parse(releases).contains { $0.version == running && $0.release.prerelease }
    }
}

/// The section of a changelog a change sits in.
public enum ChangeKind: Int, CaseIterable, Comparable, Sendable {
    case new, fixed, other

    public var title: String {
        switch self {
        case .new: "New"
        case .fixed: "Fixed"
        case .other: "Other changes"
        }
    }

    public static func < (left: ChangeKind, right: ChangeKind) -> Bool { left.rawValue < right.rawValue }
}

/// The part of the hub a change touched, as the changelog tags it.
public enum HubPart: String, CaseIterable, Sendable {
    case mac = "Mac"
    case server = "Server"
    case phone = "Phone"
}

/// One line of a release's changelog.
public struct ChangeNote: Hashable, Sendable {
    public var kind: ChangeKind
    public var parts: [HubPart]
    /// The change in words, without its tags or pull request.
    public var text: String
    public var pullRequest: Int?

    public init(kind: ChangeKind, parts: [HubPart], text: String, pullRequest: Int? = nil) {
        self.kind = kind
        self.parts = parts
        self.text = text
        self.pullRequest = pullRequest
    }

    /// "Mac", or "Mac, Server" for a change to both.
    public var partsLabel: String {
        parts.map(\.rawValue).joined(separator: ", ")
    }

    public var pullRequestURL: URL? {
        pullRequest.map(GitHubRepository.makePullRequestURL)
    }
}

/// Reads the changelog scripts/release/changelog.sh writes into each
/// release's notes (changelog_test.sh pins it):
///
///     ## New
///
///     - Mac: Suggest a second route for unanswered applications (#67)
///
///     ## Fixed
///
///     - Mac, Server: Retry board polls that time out (#71)
///
///     ## Other changes
///
///     - Server: chore: pin staticcheck in CI (#72)
public enum ReleaseNotes {
    /// The changes, in the order the notes list them. Lines outside a known
    /// section, and text that isn't a list item, are skipped.
    public static func parse(_ markdown: String) -> [ChangeNote] {
        var kind: ChangeKind?
        var notes: [ChangeNote] = []
        for rawLine in markdown.components(separatedBy: .newlines) {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.hasPrefix("#") {
                let heading = line.drop { $0 == "#" }.trimmingCharacters(in: .whitespaces)
                kind = ChangeKind.allCases.first { $0.title.caseInsensitiveCompare(heading) == .orderedSame }
                continue
            }
            guard let kind, line.hasPrefix("- ") || line.hasPrefix("* ") else { continue }
            if let note = parseItem(String(line.dropFirst(2)), kind: kind) {
                notes.append(note)
            }
        }
        return notes
    }

    /// "Mac, Server: Retry board polls that time out (#71)": the tags before
    /// the first colon, when every one names a part; the pull request at the
    /// end.
    static func parseItem(_ item: String, kind: ChangeKind) -> ChangeNote? {
        var text = item.trimmingCharacters(in: .whitespaces)
        var parts: [HubPart] = []
        if let colon = text.range(of: ": ") {
            let tags = text[..<colon.lowerBound].split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }
            let found = tags.compactMap(HubPart.init(rawValue:))
            if !found.isEmpty, found.count == tags.count {
                parts = found
                text = String(text[colon.upperBound...])
            }
        }
        var pullRequest: Int?
        if text.hasSuffix(")"), let open = text.range(of: " (#", options: .backwards),
           let number = Int(text[open.upperBound..<text.index(before: text.endIndex)]) {
            pullRequest = number
            text = String(text[..<open.lowerBound])
        }
        text = text.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return nil }
        return ChangeNote(kind: kind, parts: parts, text: text, pullRequest: pullRequest)
    }
}

/// Every change since the running version, across the releases in between:
/// what Settings › Version lists under the ready version.
public struct WhatsNew: Equatable, Sendable {
    public var releaseCount: Int
    public var notes: [ChangeNote]

    public init(releaseCount: Int, notes: [ChangeNote]) {
        self.releaseCount = releaseCount
        self.notes = notes
    }

    /// Merges the releases' notes, newest release first, grouped New, Fixed,
    /// then Other changes. A change two releases both list shows once.
    public static func merge(_ releases: [MacRelease]) -> WhatsNew {
        var seen = Set<ChangeNote>()
        let notes = releases.sorted { $0.version > $1.version }
            .flatMap(\.notes)
            .filter { seen.insert($0).inserted }
        // A stable sort by kind keeps each section's newest-first order.
        let grouped = ChangeKind.allCases.flatMap { kind in notes.filter { $0.kind == kind } }
        return WhatsNew(releaseCount: releases.count, notes: grouped)
    }

    public func getNotes(_ kind: ChangeKind) -> [ChangeNote] {
        notes.filter { $0.kind == kind }
    }

    /// New and Fixed, which show before *Show all*.
    public var highlights: [ChangeNote] {
        notes.filter { $0.kind != .other }
    }

    public var changeCount: Int { notes.count }

    /// Whether a change is for the phone, which installs there, not with
    /// this version.
    public var hasPhoneChanges: Bool {
        notes.contains { $0.parts.contains(.phone) }
    }

    /// "3 releases · 8 changes".
    public var summary: String {
        let releases = releaseCount == 1 ? "1 release" : "\(releaseCount) releases"
        let changes = changeCount == 1 ? "1 change" : "\(changeCount) changes"
        return "\(releases) · \(changes)"
    }
}

/// Reads the repository's releases from GitHub's public API, without a
/// token: the Mac app asks it directly, not through the server, so a hub
/// whose server won't start can still get the version that fixes it.
///
/// Launched with `--qa-mode`, the app reads them from HUB_RELEASES_URL
/// instead, when it holds an http(s) URL, so QA can serve a stub feed.
/// Without the flag the variable is ignored.
public struct ReleaseFeed: Sendable {
    public static let urlVariable = "HUB_RELEASES_URL"

    public enum Failure: LocalizedError, Equatable {
        case status(Int)
        case rateLimited

        public var errorDescription: String? {
            switch self {
            case let .status(code): "GitHub answered \(code)."
            case .rateLimited: "GitHub's limit for requests without a token was reached; the next check tries again."
            }
        }
    }

    private let session: URLSession
    private let url: URL
    private let appVersion: String

    /// Without a URL, the feed reads GitHub's, or QA's from
    /// HUB_RELEASES_URL in QA mode.
    public init(
        session: URLSession = .shared,
        url: URL? = nil,
        appVersion: String = HubClient.appVersion,
        arguments: [String] = ProcessInfo.processInfo.arguments,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) {
        self.session = session
        self.url = url ?? Self.makeURL(arguments: arguments, environment: environment)
        self.appVersion = appVersion
    }

    /// HUB_RELEASES_URL when the app runs in QA mode and the variable holds
    /// an http(s) URL; GitHub's releases otherwise.
    public static func makeURL(arguments: [String], environment: [String: String]) -> URL {
        guard arguments.contains(HubConnection.qaModeArgument),
              let text = environment[urlVariable]?.trimmingCharacters(in: .whitespaces),
              let url = URL(string: text), url.scheme == "http" || url.scheme == "https", url.host() != nil
        else { return GitHubRepository.releasesAPIURL }
        return url
    }

    public func fetch() async throws -> [GitHubRelease] {
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 30)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("2022-11-28", forHTTPHeaderField: "X-GitHub-Api-Version")
        request.setValue("JobSearchHub/\(appVersion)", forHTTPHeaderField: "User-Agent")
        let (data, response) = try await session.data(for: request)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard status == 200 else {
            if status == 403 || status == 429 { throw Failure.rateLimited }
            throw Failure.status(status)
        }
        return try GitHubRelease.decodeList(data)
    }
}
