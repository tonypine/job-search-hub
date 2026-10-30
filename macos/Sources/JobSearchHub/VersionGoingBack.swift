import AppKit
import JobSearchHubCore
import Observation
import UserNotifications

/// Going back to an earlier version: Settings › Version's *Go back to…* and
/// *Install an older version…*, the prompt after a crash in a version's
/// first day, and the banner when the running version was withdrawn. It
/// weighs what going back costs, shows it in a sheet over the main window,
/// and hands the going back to `hub-update` through the installer.
@MainActor
@Observable
final class GoingBack {
    static let shared = GoingBack(updates: Installer.shared.updates)
    private static let withdrawnDismissedKey = "withdrawnBannerDismissed"
    private static let withdrawnNotifiedKey = "withdrawnNotified"

    /// What the sheet asks about: its title and buttons follow.
    enum Reason: Equatable {
        /// Settings › Version's *Go back to…*.
        case settings
        /// The running version crashed in its first day.
        case crash
        /// The running version's release was withdrawn.
        case withdrawn
        /// *Install an older version…*.
        case older
    }

    /// Going back, weighed: what the sheet offers.
    struct Offer: Equatable {
        var reason: Reason
        var running: HubVersion
        var target: HubVersion
        /// The earlier version's bundle: in `previous/`, or downloaded.
        var app: URL
        var databaseMigration: Int
        /// The migration whose copy of the database is restored, or the
        /// target's own when the database didn't migrate since.
        var restoredMigration: Int
        var cost: GoingBackCost
        /// When the running version was installed, for the crash prompt.
        var installedAt: Date?
    }

    /// What the sheet shows.
    enum Page: Equatable {
        case olderVersions
        case offer
    }

    var isShowingSheet = false
    private(set) var page = Page.offer
    private(set) var offer: Offer?
    /// Weighing the cost: the sheet waits for it.
    private(set) var isWeighing = false
    /// The older version downloading before it can be offered.
    private(set) var downloading: HubVersion?
    /// Going back was confirmed: the app is about to quit.
    private(set) var isStarting = false
    var failure: HubFailure?
    private(set) var olderVersions: [OlderVersion]?
    private(set) var isLoadingOlderVersions = false
    /// Going back from the withdrawn version, weighed for its banner.
    private(set) var withdrawnOffer: Offer?
    private(set) var isWithdrawnBannerDismissed = false

    @ObservationIgnored let updates: UpdatesFolder
    @ObservationIgnored var makeClient: @MainActor () -> HubClient? = { nil }
    @ObservationIgnored private let inspector = CodesignInspector()
    @ObservationIgnored private let running = HubVersion(HubClient.appVersion)

    init(updates: UpdatesFolder) {
        self.updates = updates
    }

    /// The version in `previous/`, when it's older than the one running: the
    /// one *Go back to* offers.
    var previousVersion: HubVersion? {
        guard let running, let previous = updates.readPreviousVersion(), previous < running else { return nil }
        return previous
    }

    // MARK: Offering

    /// *Go back to 0.1.247…*, the crash prompt's and the withdrawn banner's
    /// way back: the sheet shows at once, and says the cost once weighed.
    func offerPrevious(_ reason: Reason) async {
        page = .offer
        offer = nil
        failure = nil
        isShowingSheet = true
        isWeighing = true
        defer { isWeighing = false }
        offer = await weighPrevious(reason)
        if offer == nil && failure == nil {
            isShowingSheet = false
        }
    }

    private func weighPrevious(_ reason: Reason) async -> Offer? {
        guard let running else { return nil }
        guard let app = UpdatesFolder.findApp(in: updates.previousURL),
              let target = BundleChecks.readInfo("CFBundleShortVersionString", of: app).flatMap(HubVersion.init), target < running
        else {
            failure = HubFailure("Couldn't go back", advice: "No earlier version is kept on this Mac.")
            return nil
        }
        guard let migration = await inspector.readServerBuild(of: app)?.newestMigration else {
            failure = HubFailure("Couldn't go back to \(target)", advice: "Its server doesn't say which migrations it knows.")
            return nil
        }
        return await weigh(reason, running: running, target: target, app: app, migration: migration, knownMigration: nil)
    }

    /// Weighs going back to `target`, whose server knows `migration`: the
    /// database's migration is the running server's, and when it migrated
    /// since, the hub's report says what restoring its copy loses. An older
    /// release whose copy at its own migration is gone restores the one at
    /// `knownMigration`, which its server migrates up from.
    private func weigh(_ reason: Reason, running: HubVersion, target: HubVersion, app: URL, migration: Int, knownMigration: Int?) async -> Offer? {
        guard let database = await readDatabaseMigration() else {
            failure = HubFailure("Couldn't go back to \(target)", advice: "This version's server doesn't say which migrations it knows.")
            return nil
        }
        var report: GoingBackReport?
        if migration < database, let client = makeClient() {
            report = try? await client.get("v1/version/going-back", as: GoingBackReport.self)
        }
        var restored = migration
        if migration < database, let report, report.findDump(migration: migration) == nil,
           let known = knownMigration, known < migration, report.findDump(migration: known) != nil {
            restored = known
        }
        return Offer(
            reason: reason, running: running, target: target, app: app, databaseMigration: database, restoredMigration: restored,
            cost: GoingBackCost.make(target: restored, database: database, report: report),
            installedAt: InstallRecord.findInstalledAt(running, in: updates.readInstalls())
        )
    }

    /// The database's migration: the running server's newest, which it
    /// migrated up to at its start; this bundle's when it doesn't answer.
    private func readDatabaseMigration() async -> Int? {
        if let client = makeClient(), let build = try? await client.get("v1/version", as: ServerBuild.self), let migration = build.newestMigration {
            return migration
        }
        return await inspector.readServerBuild(of: Bundle.main.bundleURL)?.newestMigration
    }

    // MARK: Older versions

    /// *Install an older version…*: the sheet lists the older releases the
    /// hub can go back to.
    func showOlderVersions() async {
        page = .olderVersions
        offer = nil
        failure = nil
        isShowingSheet = true
        await loadOlderVersions()
    }

    /// Reads the releases from GitHub and the copies of the database from
    /// the hub, which says which older versions can run on it.
    func loadOlderVersions() async {
        guard let running else { return }
        failure = nil
        guard let client = makeClient() else {
            failure = HubFailure("Couldn't list older versions", advice: "Connect to the hub first: it knows which copies of the database it keeps.")
            return
        }
        isLoadingOlderVersions = true
        defer { isLoadingOlderVersions = false }
        do {
            async let releases = ReleaseFeed().fetch()
            async let report = client.get("v1/version/going-back", as: GoingBackReport.self)
            let (fetched, read) = try await (releases, report)
            olderVersions = OlderVersions.pick(fetched, running: running, previous: previousVersion, bad: updates.readBadVersions(), report: read)
        } catch {
            failure = HubFailure("Couldn't list older versions", error)
        }
    }

    /// Downloads and checks an older version, as a new one is, then weighs
    /// going back to it.
    func offerOlder(_ older: OlderVersion) async {
        guard let running, downloading == nil else { return }
        failure = nil
        downloading = older.version
        defer { downloading = nil }
        let checked: CheckedVersion
        if let earlier = updates.readChecked(older.version) {
            checked = earlier
        } else {
            do throws(DownloadProblem) {
                checked = try await NewVersionChecker.shared.download(older.release)
            } catch {
                updates.removeFolder(for: older.version)
                updates.log("\(older.version) \(error.title): \(error.explanation)", at: .now)
                failure = HubFailure("Couldn't download \(older.version)", advice: "It \(error.title). \(error.explanation)")
                return
            }
        }
        guard let migration = await inspector.readServerBuild(of: checked.app)?.newestMigration else {
            failure = HubFailure("Couldn't go back to \(older.version)", advice: "Its server doesn't say which migrations it knows.")
            return
        }
        guard let offer = await weigh(.older, running: running, target: older.version, app: checked.app, migration: migration, knownMigration: older.migration)
        else { return }
        self.offer = offer
        page = .offer
    }

    // MARK: Going back

    /// *Go back*: `hub-update` takes it from here, and the app quits.
    func confirm() {
        guard let offer, offer.cost.isPossible else { return }
        let state = InstallState.makeGoingBack(
            from: offer.running, to: offer.target, installed: ServerLaunchAgent.makeInstalledAppURL(home: FileManager.default.homeDirectoryForCurrentUser),
            app: offer.app, databaseMigration: offer.databaseMigration, restoredMigration: offer.restoredMigration, startedAt: .now,
            jobPlist: Installer.jobPlistURL
        )
        isStarting = true
        do {
            try Installer.shared.goBack(state)
        } catch {
            isStarting = false
            failure = HubFailure("Couldn't go back to \(offer.target)", error)
        }
    }

    /// *Cancel*, or *Stay on 0.1.252*.
    func dismiss() {
        isShowingSheet = false
        offer = nil
        failure = nil
    }

    /// From the offer back to the list of older versions.
    func showList() {
        offer = nil
        failure = nil
        page = .olderVersions
    }

    // MARK: A crash in the first day

    /// At launch: whether the last run of this version, installed in the
    /// last day, crashed or didn't quit cleanly, which asks to go back; then
    /// this run's record, which a clean quit marks.
    func readLaunch() {
        guard let running else { return }
        let previousRun = updates.readRun()
        let crashed = FirstDayCrash.didCrash(
            running: running, installedAt: InstallRecord.findInstalledAt(running, in: updates.readInstalls()), previousRun: previousRun,
            crashReports: Self.readCrashReports(since: previousRun?.startedAt ?? .distantPast), now: .now
        )
        try? updates.writeRun(RunRecord(version: running.description, startedAt: .now, quitCleanly: false))
        guard crashed, previousVersion != nil else { return }
        updates.log("Job Search Hub \(running) quit unexpectedly in its first day.", at: .now)
        Task {
            // The token comes from the Keychain shortly after launch; the
            // cost needs the hub.
            for _ in 0..<20 where makeClient() == nil {
                try? await Task.sleep(for: .milliseconds(500))
            }
            await offerPrevious(.crash)
        }
    }

    /// The app quits normally: the next launch knows this run didn't crash.
    func markCleanQuit() {
        guard let running, var record = updates.readRun(), record.version == running.description else { return }
        record.quitCleanly = true
        try? updates.writeRun(record)
    }

    /// The app's crash reports written since `since`, from their headers.
    nonisolated static func readCrashReports(since: Date) -> [CrashReport] {
        let folder = FileManager.default.homeDirectoryForCurrentUser.appending(path: "Library/Logs/DiagnosticReports")
        let names = (try? FileManager.default.contentsOfDirectory(atPath: folder.path)) ?? []
        return names.filter(CrashReport.isReportName).compactMap { name in
            let file = folder.appending(path: name)
            guard let modified = try? file.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate, modified >= since,
                  let handle = try? FileHandle(forReadingFrom: file)
            else { return nil }
            defer { try? handle.close() }
            let head = (try? handle.read(upToCount: 4096)) ?? Data()
            return CrashReport.parse(String(decoding: head, as: UTF8.self))
        }
    }

    // MARK: A withdrawn version

    /// The running version's release was withdrawn: the banner weighs going
    /// back to the previous version, and a notification says so once, when
    /// the app isn't in front.
    func noticeWithdrawn() async {
        guard let running else { return }
        isWithdrawnBannerDismissed = UserDefaults.standard.string(forKey: Self.withdrawnDismissedKey) == running.description
        if previousVersion != nil {
            let failure = self.failure
            withdrawnOffer = await weighPrevious(.withdrawn)
            // Weighing for the banner shows no failure of its own.
            self.failure = failure
        } else {
            withdrawnOffer = nil
        }
        guard UserDefaults.standard.string(forKey: Self.withdrawnNotifiedKey) != running.description, !NSApp.isActive else { return }
        UserDefaults.standard.set(running.description, forKey: Self.withdrawnNotifiedKey)
        let content = UNMutableNotificationContent()
        content.title = "\(running) was withdrawn"
        content.body = withdrawnOffer.map { "\($0.cost.summarize(target: $0.target)) Open Settings › Version to go back." }
            ?? "It was pulled after its release. The next release replaces it."
        content.sound = .default
        try? await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "withdrawn-\(running)", content: content, trigger: nil))
    }

    func dismissWithdrawnBanner() {
        guard let running else { return }
        UserDefaults.standard.set(running.description, forKey: Self.withdrawnDismissedKey)
        isWithdrawnBannerDismissed = true
    }
}
