import Foundation

/// What `GET /v1/version/going-back` answers: the migration the database is
/// at, the release that first migrated it to each one, and each copy of the
/// database taken before a migration, with what was written since, which
/// going back to it would lose.
public struct GoingBackReport: Decodable, Equatable, Sendable {
    public var migration: Int
    public var migrationReleases: [MigrationRelease]
    /// The oldest first.
    public var dumps: [Dump]

    public struct MigrationRelease: Decodable, Equatable, Sendable {
        public var migration: Int
        public var release: String

        public init(migration: Int, release: String) {
            self.migration = migration
            self.release = release
        }
    }

    /// A copy of the database at `migration`, taken before the version after
    /// it migrated it.
    public struct Dump: Decodable, Equatable, Sendable {
        public var migration: Int
        public var dumpedAt: Date
        /// When the version after it began writing; nil without its mark.
        public var since: Date?
        /// What was written since, the most first.
        public var lost: [LostCount]
        /// Why what was written since can't be counted; nil when it can.
        public var uncountable: String?

        public init(migration: Int, dumpedAt: Date, since: Date?, lost: [LostCount], uncountable: String? = nil) {
            self.migration = migration
            self.dumpedAt = dumpedAt
            self.since = since
            self.lost = lost
            self.uncountable = uncountable
        }
    }

    public init(migration: Int, migrationReleases: [MigrationRelease], dumps: [Dump]) {
        self.migration = migration
        self.migrationReleases = migrationReleases
        self.dumps = dumps
    }

    /// The migration a release knows at least: the newest one that a
    /// release no newer than it first migrated the database to. It may know
    /// more, which its server applies after the restore. Nil when no release
    /// that old recorded one.
    public func findMigration(of version: HubVersion) -> Int? {
        migrationReleases.filter { HubVersion($0.release).map { $0 <= version } ?? false }.map(\.migration).max()
    }

    /// The newest copy of the database at `migration`.
    public func findDump(migration: Int) -> Dump? {
        dumps.last { $0.migration == migration }
    }
}

/// One kind of write going back would lose, counted: 3 "jobs found".
public struct LostCount: Codable, Equatable, Hashable, Sendable {
    public var rows: Int
    public var words: String

    public init(rows: Int, words: String) {
        self.rows = rows
        self.words = words
    }

    /// "3 jobs found".
    public var description: String { "\(rows) \(words)" }
}

/// What going back to an earlier version costs.
public enum GoingBackCost: Equatable, Sendable {
    /// The database didn't change since that version: an install in
    /// reverse, which loses nothing.
    case nothingLost
    /// The database goes back to its copy from before the next version
    /// changed it, and what was written since is lost; nothing when empty.
    case loses(since: Date, lost: [LostCount])
    /// The database goes back to its copy, but what was written since can't
    /// be counted: all of it would be lost. Nil when the hub couldn't be
    /// asked.
    case uncountable(since: Date?)
    /// The copy of the database from before is gone, so the earlier server
    /// couldn't run: going back isn't possible.
    case noCopy

    /// What going back to a version whose newest migration is `target`
    /// costs, the database being at `database`. Without the hub's report,
    /// `hub-update` still counts once the server has stopped.
    public static func make(target: Int, database: Int, report: GoingBackReport?) -> GoingBackCost {
        if target >= database { return .nothingLost }
        guard let report else { return .uncountable(since: nil) }
        guard let dump = report.findDump(migration: target) else { return .noCopy }
        let since = dump.since ?? dump.dumpedAt
        if dump.uncountable != nil { return .uncountable(since: since) }
        return .loses(since: since, lost: dump.lost)
    }

    public var isPossible: Bool { self != .noCopy }

    /// Whether going back loses anything the owner would miss, or might.
    public var losesWrites: Bool {
        switch self {
        case .nothingLost, .noCopy: false
        case let .loses(_, lost): !lost.isEmpty
        case .uncountable: true
        }
    }

    /// Whether the database goes back with the version.
    public var restoresDatabase: Bool {
        switch self {
        case .loses, .uncountable: true
        case .nothingLost, .noCopy: false
        }
    }

    /// What the database does, in a sentence: "Going back also takes the
    /// database back to today, 14:02, before 0.1.252 changed it."
    public func describeDatabase(running: HubVersion, target: HubVersion, now: Date, calendar: Calendar = .current) -> String {
        switch self {
        case .nothingLost:
            return "\(running) didn't change the database, so going back to \(target) loses nothing."
        case let .loses(since, _), let .uncountable(since?):
            return "Going back also takes the database back to \(Self.describeTime(since, now: now, calendar: calendar)), before \(running) changed it."
        case .uncountable(nil):
            return "Going back also takes the database back to before \(running) changed it."
        case .noCopy:
            return "The copy of the database from before \(running) changed it is gone, so the hub can't go back to \(target)."
        }
    }

    /// What would be lost, in a sentence, or nil when nothing would:
    /// "Since then: 2 jobs found, 1 card moved. These would be lost."
    public var lostSentence: String? {
        switch self {
        case let .loses(_, lost) where !lost.isEmpty:
            "Since then: \(lost.map(\.description).joined(separator: ", ")). These would be lost."
        case .loses:
            "Nothing was written since, so nothing would be lost."
        case .uncountable:
            "What was written since can't be counted, and all of it would be lost."
        case .nothingLost, .noCopy:
            nil
        }
    }

    /// The banner's half sentence: "Going back to 0.1.247 loses nothing."
    public func summarize(target: HubVersion) -> String {
        switch self {
        case .nothingLost:
            "Going back to \(target) loses nothing."
        case let .loses(_, lost):
            lost.isEmpty ? "Going back to \(target) loses nothing." : "Going back to \(target) loses \(lost.map(\.description).joined(separator: ", "))."
        case .uncountable:
            "Going back to \(target) loses what was written since, which can't be counted."
        case .noCopy:
            "The hub can't go back to \(target): the copy of the database from before is gone."
        }
    }

    /// "today, 14:02", "yesterday, 09:30", or "3 Oct, 14:02".
    public static func describeTime(_ date: Date, now: Date, calendar: Calendar = .current) -> String {
        let time = DateFormatter()
        time.locale = Locale(identifier: "en_US_POSIX")
        time.timeZone = calendar.timeZone
        time.dateFormat = "HH:mm"
        let days = calendar.dateComponents([.day], from: calendar.startOfDay(for: date), to: calendar.startOfDay(for: now)).day ?? 0
        switch days {
        case 0: return "today, \(time.string(from: date))"
        case 1: return "yesterday, \(time.string(from: date))"
        default:
            let day = DateFormatter()
            day.locale = Locale(identifier: "en_GB")
            day.timeZone = calendar.timeZone
            day.dateFormat = "d MMM"
            return "\(day.string(from: date)), \(time.string(from: date))"
        }
    }
}

/// An older release *Install an older version…* lists, with what going back
/// to it would cost.
public struct OlderVersion: Equatable, Sendable {
    public var release: MacRelease
    /// The migration it knows at least, whose copy of the database going
    /// back restores.
    public var migration: Int
    public var cost: GoingBackCost

    public var version: HubVersion { release.version }

    public init(release: MacRelease, migration: Int, cost: GoingBackCost) {
        self.release = release
        self.migration = migration
        self.cost = cost
    }
}

public enum OlderVersions {
    /// The older releases the app can go back to, newest first: offered (not
    /// withdrawn), older than the running version, not the previous one,
    /// which *Go back to* offers, not one that failed or was left on this
    /// Mac, with an app to download, and listed only when they need no copy
    /// of the database or theirs is still on disk.
    public static func pick(
        _ releases: [GitHubRelease], running: HubVersion, previous: HubVersion?, bad: Set<HubVersion>, report: GoingBackReport
    ) -> [OlderVersion] {
        MacReleases.getOffered(releases).compactMap { release in
            guard release.version < running, release.version != previous, !bad.contains(release.version),
                  release.zipAsset != nil, release.checksumAsset != nil,
                  let migration = report.findMigration(of: release.version)
            else { return nil }
            let cost = GoingBackCost.make(target: migration, database: report.migration, report: report)
            guard cost.isPossible else { return nil }
            return OlderVersion(release: release, migration: migration, cost: cost)
        }
    }
}

/// The app's record of its own run, `Updates/run.json`: written at launch,
/// and marked clean when it quits normally.
public struct RunRecord: Codable, Equatable, Sendable {
    public var version: String
    public var startedAt: Date
    public var quitCleanly: Bool

    public init(version: String, startedAt: Date, quitCleanly: Bool) {
        self.version = version
        self.startedAt = startedAt
        self.quitCleanly = quitCleanly
    }
}

/// A crash report macOS wrote for the app, in
/// `~/Library/Logs/DiagnosticReports/JobSearchHub-*.ips`.
public struct CrashReport: Equatable, Sendable {
    public var version: String
    public var crashedAt: Date

    public init(version: String, crashedAt: Date) {
        self.version = version
        self.crashedAt = crashedAt
    }

    /// Whether a file in DiagnosticReports is the app's crash report.
    public static func isReportName(_ name: String) -> Bool {
        name.hasPrefix("JobSearchHub-") && name.hasSuffix(".ips")
    }

    /// Reads an `.ips` report's first line, its JSON header, which names
    /// the app's version and when it crashed:
    /// `{"app_name":"JobSearchHub","timestamp":"2026-10-06 14:02:11.00 -0300","app_version":"0.1.252",…}`.
    public static func parse(_ text: String) -> CrashReport? {
        guard let line = text.split(separator: "\n", maxSplits: 1).first,
              let header = try? JSONSerialization.jsonObject(with: Data(line.utf8)) as? [String: Any],
              let version = header["app_version"] as? String, let timestamp = header["timestamp"] as? String
        else { return nil }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        for format in ["yyyy-MM-dd HH:mm:ss.SS Z", "yyyy-MM-dd HH:mm:ss Z"] {
            formatter.dateFormat = format
            if let date = formatter.date(from: timestamp) {
                return CrashReport(version: version, crashedAt: date)
            }
        }
        return nil
    }
}

/// An install as `hub-update` records it in `Updates/installs.json`; going
/// back is recorded as an install of the earlier version.
public struct InstallRecord: Codable, Equatable, Sendable {
    public var from: String
    public var to: String
    public var dump: String?
    public var at: Date
    public var wentBack: Bool?

    public init(from: String, to: String, dump: String? = nil, at: Date, wentBack: Bool? = nil) {
        self.from = from
        self.to = to
        self.dump = dump
        self.at = at
        self.wentBack = wentBack
    }

    /// When `version` was last put in place, by an install or going back.
    public static func findInstalledAt(_ version: HubVersion, in installs: [InstallRecord]) -> Date? {
        installs.filter { $0.to == version.description }.map(\.at).max()
    }
}

/// Whether the version running crashed in its first day: on launch, the app
/// asks to go back when the last run of a version installed in the last 24
/// hours ended in a crash report, or without a clean quit.
public enum FirstDayCrash {
    public static let firstDay: TimeInterval = 24 * 60 * 60

    public static func didCrash(
        running: HubVersion, installedAt: Date?, previousRun: RunRecord?, crashReports: [CrashReport], now: Date
    ) -> Bool {
        // The first run of a version starts before hub-update records its
        // install, once that run has opened: its start isn't compared.
        guard let installedAt, now.timeIntervalSince(installedAt) < firstDay,
              let previousRun, previousRun.version == running.description
        else { return false }
        if !previousRun.quitCleanly { return true }
        return crashReports.contains { $0.version == running.description && $0.crashedAt >= previousRun.startedAt }
    }
}
