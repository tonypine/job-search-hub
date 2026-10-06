import Foundation

/// A downloaded version that passed its checks, with what's in it: what the
/// sidebar's label and Settings › Version offer.
public struct ReadyVersion: Equatable, Sendable {
    public var version: HubVersion
    public var whatsNew: WhatsNew
    /// Whether installing it migrates the database; nil when unknown.
    public var changesDatabase: Bool?
    public var releaseURL: URL?

    public init(version: HubVersion, whatsNew: WhatsNew, changesDatabase: Bool?, releaseURL: URL? = nil) {
        self.version = version
        self.whatsNew = whatsNew
        self.changesDatabase = changesDatabase
        self.releaseURL = releaseURL
    }

    /// What installing it will do, in a line.
    public var installSummary: String {
        var sentences = ["The server restarts for about 10 seconds."]
        switch changesDatabase {
        case true?: sentences.append("This version changes the database, so a copy is saved first.")
        case false?: sentences.append("The database stays as it is.")
        case nil: sentences.append("If this version changes the database, a copy is saved first.")
        }
        if whatsNew.hasPhoneChanges {
            sentences.append("The phone changes install on the phone.")
        }
        return sentences.joined(separator: " ")
    }
}

/// A newer version whose download failed a check, which isn't offered.
public struct FailedVersion: Equatable, Sendable {
    public var version: HubVersion
    public var problem: DownloadProblem

    public init(version: HubVersion, problem: DownloadProblem) {
        self.version = version
        self.problem = problem
    }

    /// "0.1.252 didn't pass its checks".
    public var title: String {
        switch problem {
        case .missingAssets, .downloadFailed: "\(version) couldn't be downloaded"
        default: "\(version) didn't pass its checks"
        }
    }
}

/// What the app knows about new versions, from which Settings › Version and
/// the sidebar read their state.
public struct NewVersionFacts: Equatable, Sendable {
    public var running: HubVersion
    public var isChecking: Bool
    /// The last check that reached GitHub.
    public var lastCheckedAt: Date?
    /// The first of the checks that failed since the last one that didn't.
    public var failingSince: Date?
    /// The newest release offered, newer or not.
    public var newestRelease: HubVersion?
    public var ready: ReadyVersion?
    public var failed: FailedVersion?
    public var isRunningWithdrawn: Bool

    public init(
        running: HubVersion, isChecking: Bool = false, lastCheckedAt: Date? = nil, failingSince: Date? = nil,
        newestRelease: HubVersion? = nil, ready: ReadyVersion? = nil, failed: FailedVersion? = nil, isRunningWithdrawn: Bool = false
    ) {
        self.running = running
        self.isChecking = isChecking
        self.lastCheckedAt = lastCheckedAt
        self.failingSince = failingSince
        self.newestRelease = newestRelease
        self.ready = ready
        self.failed = failed
        self.isRunningWithdrawn = isRunningWithdrawn
    }
}

/// The state Settings › Version opens with, above the ready version.
public enum NewVersionStatus: Equatable, Sendable {
    /// Asking GitHub, or downloading and checking what it found.
    case checking
    /// The running version is the newest release.
    case upToDate(newest: HubVersion?, checkedAt: Date?)
    /// A build from a checkout, which no version installs over by itself.
    case localBuild(commit: String?)
    /// A newer version is ready; Settings › Version shows it in full.
    case ready
    /// The newest version failed a check, and wasn't offered.
    case failedChecks(FailedVersion)
    /// No check has reached GitHub for a day.
    case cannotCheck(since: Date)
    /// The running version's release was withdrawn.
    case withdrawn(HubVersion)

    /// How long checks can fail before the app says so.
    public static let staleAfter: TimeInterval = 24 * 60 * 60

    /// The state that matters most: a check under way, then a withdrawn
    /// version, a newer download that failed its checks, a day without a
    /// check, a local build, then a ready version or none.
    public static func make(_ facts: NewVersionFacts, now: Date) -> NewVersionStatus {
        if facts.isChecking {
            return .checking
        }
        if facts.isRunningWithdrawn {
            return .withdrawn(facts.running)
        }
        if let failed = facts.failed, failed.version > (facts.ready?.version ?? facts.running) {
            return .failedChecks(failed)
        }
        if let since = facts.failingSince, now.timeIntervalSince(since) >= staleAfter {
            return .cannotCheck(since: since)
        }
        if facts.running.isLocalBuild {
            return .localBuild(commit: facts.running.localCommit)
        }
        if facts.ready != nil {
            return .ready
        }
        return .upToDate(newest: facts.newestRelease, checkedAt: facts.lastCheckedAt)
    }
}
