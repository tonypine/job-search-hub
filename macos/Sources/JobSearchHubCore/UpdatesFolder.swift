import Foundation

/// `~/Library/Application Support/JobSearchHub/Updates/`: one folder per
/// downloaded version, `<version>/`, holding the unpacked app and a record
/// of its checks; `previous/`, the version installed before this one; and
/// `install.log`. It keeps one downloaded version: a newer one that passes
/// its checks replaces an older one that wasn't installed.
public struct UpdatesFolder: Sendable {
    public let root: URL

    /// The folder, and the log, that *Show install log* opens.
    public static func makeDefault(home: URL) -> UpdatesFolder {
        UpdatesFolder(root: home.appending(path: "Library/Application Support/JobSearchHub/Updates"))
    }

    public init(root: URL) {
        self.root = root
    }

    /// Folders and files the downloads never replace.
    static let kept: Set<String> = [
        "previous", "install.log", "state.json", "last-install.json", "launched", "bad-versions.json", "reopen.json", "hub-update",
        "hub-update.log", "installs.json",
    ]
    static let recordName = "checked.json"

    public var logURL: URL { root.appending(path: "install.log") }
    public var previousURL: URL { root.appending(path: "previous") }

    public func makeFolderURL(for version: HubVersion) -> URL {
        root.appending(path: version.description)
    }

    /// An empty folder for the version's download, replacing one left from
    /// before.
    public func prepareFolder(for version: HubVersion) throws -> URL {
        let folder = makeFolderURL(for: version)
        try? FileManager.default.removeItem(at: folder)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        return folder
    }

    public func removeFolder(for version: HubVersion) {
        try? FileManager.default.removeItem(at: makeFolderURL(for: version))
    }

    /// Deletes every downloaded version but `version`, or all of them; what
    /// isn't a download stays.
    public func removeDownloads(keeping version: HubVersion?) {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: root.path)) ?? []
        for name in names where !Self.kept.contains(name) && name != version?.description {
            try? FileManager.default.removeItem(at: root.appending(path: name))
        }
    }

    /// What `checked.json` records once a download passed its checks.
    struct Record: Codable {
        var version: String
        var app: String
        var changesDatabase: Bool?
        var checkedAt: Date
    }

    /// Records that the version's app passed its checks, so the next launch
    /// offers it without downloading it again.
    public func recordChecked(_ checked: CheckedVersion, at date: Date) throws {
        let record = Record(
            version: checked.version.description, app: checked.app.lastPathComponent, changesDatabase: checked.changesDatabase, checkedAt: date
        )
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        try encoder.encode(record).write(to: makeFolderURL(for: checked.version).appending(path: Self.recordName), options: .atomic)
    }

    /// The version's download, when an earlier check passed it and its app
    /// is still there.
    public func readChecked(_ version: HubVersion) -> CheckedVersion? {
        let folder = makeFolderURL(for: version)
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        guard let data = try? Data(contentsOf: folder.appending(path: Self.recordName)),
              let record = try? decoder.decode(Record.self, from: data), record.version == version.description
        else { return nil }
        let app = folder.appending(path: record.app)
        guard FileManager.default.fileExists(atPath: app.appending(path: "Contents/Info.plist").path) else { return nil }
        return CheckedVersion(version: version, app: app, changesDatabase: record.changesDatabase)
    }

    /// The app a download unpacked to: the first `.app` in the folder.
    public static func findApp(in folder: URL) -> URL? {
        let names = (try? FileManager.default.contentsOfDirectory(atPath: folder.path)) ?? []
        return names.sorted().first { $0.hasSuffix(".app") }.map { folder.appending(path: $0) }
    }

    /// The version kept in `previous/`, for going back, when there is one.
    public func readPreviousVersion() -> HubVersion? {
        guard let app = Self.findApp(in: previousURL),
              let text = BundleChecks.readInfo("CFBundleShortVersionString", of: app)
        else { return nil }
        return HubVersion(text)
    }

    /// Adds a line to `install.log`, stamped with the time.
    public func log(_ line: String, at date: Date) {
        try? FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let entry = Data("\(date.formatted(.iso8601)) \(line)\n".utf8)
        if let handle = try? FileHandle(forWritingTo: logURL) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: entry)
        } else {
            try? entry.write(to: logURL)
        }
    }
}
