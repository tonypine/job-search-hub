import Foundation
import JobSearchHubCore
import Observation

/// Finds new versions of the app: it asks GitHub for the Mac releases at
/// launch and every hour, downloads the newest one above its own into
/// `Updates/<version>/`, and checks it before anything shows. A version
/// that's still downloading is never offered, so the owner never waits on
/// one. Installing it is the installer's job.
@MainActor
@Observable
final class NewVersionChecker {
    static let shared = NewVersionChecker()
    static let checkInterval: Duration = .seconds(60 * 60)
    private static let lastCheckedKey = "newVersionLastChecked"
    private static let failingSinceKey = "newVersionFailingSince"

    private(set) var facts: NewVersionFacts
    /// The running server's build, from its `GET /v1/version`.
    private(set) var runningServer: ServerBuild?
    /// When the running app was installed: its bundle's creation.
    let installedAt: Date?
    private(set) var previousVersion: HubVersion?
    private(set) var hasInstallLog = false
    /// The running version's release page, when it's a release.
    private(set) var runningReleaseURL: URL?

    @ObservationIgnored let updates = UpdatesFolder.makeDefault(home: FileManager.default.homeDirectoryForCurrentUser)
    @ObservationIgnored private let feed = ReleaseFeed()
    @ObservationIgnored private let inspector = CodesignInspector()
    @ObservationIgnored private var runningSigning: BundleSigning?
    @ObservationIgnored private var loop: Task<Void, Never>?
    @ObservationIgnored private var check: Task<Void, Never>?

    init() {
        let running = HubVersion(HubClient.appVersion) ?? HubVersion("0.1.0-dev")!
        let defaults = UserDefaults.standard
        facts = NewVersionFacts(
            running: running,
            lastCheckedAt: defaults.object(forKey: Self.lastCheckedKey) as? Date,
            failingSince: defaults.object(forKey: Self.failingSinceKey) as? Date
        )
        installedAt = try? Bundle.main.bundleURL.resourceValues(forKeys: [.creationDateKey]).creationDate
        readLocalState()
    }

    var status: NewVersionStatus {
        NewVersionStatus.make(facts, now: .now)
    }

    /// Checks at launch, then every hour, until the app quits.
    func start() {
        guard loop == nil else { return }
        loop = Task {
            while !Task.isCancelled {
                await checkAndWait()
                try? await Task.sleep(for: Self.checkInterval)
            }
        }
    }

    /// Checks now, as Check for New Version… and *Check now* ask; a check
    /// already under way is the one.
    func checkNow() {
        Task { await checkAndWait() }
    }

    private func checkAndWait() async {
        if let check {
            await check.value
            return
        }
        let task = Task { await performCheck() }
        check = task
        await task.value
        check = nil
    }

    private func performCheck() async {
        facts.isChecking = true
        defer {
            facts.isChecking = false
            readLocalState()
        }
        runningServer = await readRunningServer()

        let releases: [GitHubRelease]
        do {
            releases = try await feed.fetch()
        } catch {
            facts.failingSince = facts.failingSince ?? .now
            UserDefaults.standard.set(facts.failingSince, forKey: Self.failingSinceKey)
            updates.log("Couldn't check for new versions: \(error.localizedDescription)", at: .now)
            return
        }
        facts.lastCheckedAt = .now
        facts.failingSince = nil
        UserDefaults.standard.set(facts.lastCheckedAt, forKey: Self.lastCheckedKey)
        UserDefaults.standard.removeObject(forKey: Self.failingSinceKey)

        let running = facts.running
        let wasWithdrawn = facts.isRunningWithdrawn
        facts.isRunningWithdrawn = MacReleases.isWithdrawn(running, in: releases)
        if facts.isRunningWithdrawn && !wasWithdrawn {
            updates.log("\(running), the running version, was withdrawn.", at: .now)
        }
        facts.newestRelease = MacReleases.getOffered(releases).first?.version
        runningReleaseURL = MacReleases.parse(releases).first { $0.version == running }?.release.htmlURL

        guard let newest = MacReleases.findNewest(in: releases, above: running) else {
            facts.ready = nil
            facts.failed = nil
            updates.removeDownloads(keeping: nil)
            return
        }
        let whatsNew = WhatsNew.merge(MacReleases.findBetween(in: releases, running: running, through: newest.version))
        if facts.ready?.version == newest.version {
            facts.ready?.whatsNew = whatsNew
            return
        }
        // A version refused once isn't downloaded again until the next
        // launch, unless only the download failed.
        if let failed = facts.failed, failed.version == newest.version, !failed.problem.isDownloadFailure {
            return
        }

        let checked: CheckedVersion
        if let earlier = updates.readChecked(newest.version) {
            checked = earlier
        } else {
            do throws(DownloadProblem) {
                checked = try await download(newest)
            } catch {
                updates.removeFolder(for: newest.version)
                facts.failed = FailedVersion(version: newest.version, problem: error)
                updates.log("\(newest.version) \(error.title): \(error.explanation)", at: .now)
                return
            }
            updates.log("\(newest.version) passed its checks and is ready to install.", at: .now)
        }
        updates.removeDownloads(keeping: newest.version)
        facts.failed = nil
        facts.ready = ReadyVersion(
            version: newest.version, whatsNew: whatsNew, changesDatabase: checked.changesDatabase, releaseURL: newest.release.htmlURL
        )
    }

    /// Downloads the release's zip and checksum, then runs the checks in
    /// order: the checksum, `ditto`'s unpacking, then the bundle's.
    private func download(_ release: MacRelease) async throws(DownloadProblem) -> CheckedVersion {
        guard let zipAsset = release.zipAsset, let checksumAsset = release.checksumAsset else { throw .missingAssets }
        let folder: URL
        do {
            folder = try updates.prepareFolder(for: release.version)
        } catch {
            throw .downloadFailed(error.localizedDescription)
        }
        let zip = folder.appending(path: zipAsset.name)
        let checksumText: String
        do {
            let (downloaded, response) = try await URLSession.shared.download(from: zipAsset.browserDownloadURL)
            try Self.requireSuccess(response)
            try FileManager.default.moveItem(at: downloaded, to: zip)
            let (checksum, checksumResponse) = try await URLSession.shared.data(from: checksumAsset.browserDownloadURL)
            try Self.requireSuccess(checksumResponse)
            checksumText = String(decoding: checksum, as: UTF8.self)
        } catch {
            throw .downloadFailed(error.localizedDescription)
        }

        try Checksum.verify(zip, against: checksumText)

        let unpacked = await ProcessRunner.run(URL(filePath: "/usr/bin/ditto"), arguments: ["-x", "-k", zip.path, folder.path])
        try? FileManager.default.removeItem(at: zip)
        guard unpacked.status == 0 else { throw .noApp(unpacked.output) }
        guard let app = UpdatesFolder.findApp(in: folder) else { throw .noApp("The zip holds no .app.") }

        let running = await readRunningSigning()
        let checked = try await BundleChecks.check(
            app, expected: release.version, running: running, runningServer: runningServer, inspector: inspector
        )
        try? updates.recordChecked(checked, at: .now)
        return checked
    }

    private static func requireSuccess(_ response: URLResponse) throws {
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard status == 200 else { throw ReleaseFeed.Failure.status(status) }
    }

    /// The running app's signature, read once: every download is matched
    /// against it.
    private func readRunningSigning() async -> BundleSigning {
        if let runningSigning { return runningSigning }
        let signing = await inspector.readSigning(of: Bundle.main.bundleURL)
        runningSigning = signing
        return signing
    }

    /// The running server's version and newest migration, from `GET
    /// /v1/version`, which needs no token; this bundle's `hub-server
    /// --version` when the server doesn't answer.
    private func readRunningServer() async -> ServerBuild? {
        let hubURL = UserDefaults.standard.string(forKey: "hubURL").flatMap(URL.init(string:)) ?? URL(string: HubConnection.defaultHubURL)!
        var request = URLRequest(url: hubURL.appending(path: "v1/version"), timeoutInterval: 5)
        request.setValue("macos/\(HubClient.appVersion)", forHTTPHeaderField: HubClient.clientHeader)
        if let (data, response) = try? await URLSession.shared.data(for: request),
           (response as? HTTPURLResponse)?.statusCode == 200,
           let build = try? HubJSON.makeDecoder().decode(ServerBuild.self, from: data) {
            return build
        }
        return await inspector.readServerBuild(of: Bundle.main.bundleURL)
    }

    /// What's on disk: the version in `previous/`, the install log, and a
    /// download an earlier launch checked.
    private func readLocalState() {
        previousVersion = updates.readPreviousVersion()
        hasInstallLog = FileManager.default.fileExists(atPath: updates.logURL.path)
    }
}

private extension DownloadProblem {
    /// Whether only the download failed, which the next check tries again.
    var isDownloadFailure: Bool {
        if case .downloadFailed = self { return true }
        return false
    }
}
