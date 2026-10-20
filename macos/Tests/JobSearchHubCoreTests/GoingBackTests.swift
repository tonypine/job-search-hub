import Foundation
@testable import JobSearchHubCore
import Testing

// Made-up versions and counts only.

private func version(_ text: String) -> HubVersion {
    HubVersion(text)!
}

private let utc: Calendar = {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC")!
    return calendar
}()

/// Tuesday 6 October 2026, 16:00 UTC.
private let now = Date(timeIntervalSince1970: 1_791_302_400)
private let lost = [LostCount(rows: 3, words: "jobs found"), LostCount(rows: 1, words: "card moved"), LostCount(rows: 2, words: "people added")]

private func makeRelease(_ tag: String, prerelease: Bool = false, assets: Bool = true) -> GitHubRelease {
    let name = tag.replacingOccurrences(of: "mac-v", with: "Job-Search-Hub-")
    return GitHubRelease(
        tagName: tag, prerelease: prerelease,
        assets: assets ? [
            .init(name: "\(name).zip", browserDownloadURL: URL(string: "https://example.com/\(name).zip")!),
            .init(name: "\(name).zip.sha256", browserDownloadURL: URL(string: "https://example.com/\(name).zip.sha256")!),
        ] : []
    )
}

/// The database at 91: 0.1.240 first ran at 85, 0.1.247 at 88, 0.1.250 at
/// 90 and 0.1.252 at 91. The copies from before 0.1.247 (at 85) and 0.1.252
/// (at 90) are on disk; the one from before 0.1.250 (at 88) was pruned.
private let report = GoingBackReport(
    migration: 91,
    migrationReleases: [
        .init(migration: 85, release: "0.1.240"), .init(migration: 88, release: "0.1.247"),
        .init(migration: 90, release: "0.1.250"), .init(migration: 91, release: "0.1.252"),
    ],
    dumps: [
        .init(migration: 85, dumpedAt: now.addingTimeInterval(-5 * 86400), since: nil, lost: [], uncountable: "nothing marked when the version after it began writing"),
        .init(migration: 90, dumpedAt: now.addingTimeInterval(-7200), since: now.addingTimeInterval(-7080), lost: lost),
    ]
)

@Test func goingBackWithoutAMigrationSinceLosesNothing() {
    let cost = GoingBackCost.make(target: 91, database: 91, report: nil)
    #expect(cost == .nothingLost)
    #expect(!cost.losesWrites && !cost.restoresDatabase && cost.isPossible)
    #expect(cost.describeDatabase(running: version("0.1.252"), target: version("0.1.251"), now: now, calendar: utc)
        == "0.1.252 didn't change the database, so going back to 0.1.251 loses nothing.")
    #expect(cost.lostSentence == nil)
    #expect(cost.summarize(target: version("0.1.251")) == "Going back to 0.1.251 loses nothing.")
}

@Test func goingBackOverAMigrationCountsWhatWasWrittenSinceItsCopy() {
    let cost = GoingBackCost.make(target: 90, database: 91, report: report)
    #expect(cost == .loses(since: now.addingTimeInterval(-7080), lost: lost))
    #expect(cost.losesWrites && cost.restoresDatabase)
    #expect(cost.describeDatabase(running: version("0.1.252"), target: version("0.1.250"), now: now, calendar: utc)
        == "Going back also takes the database back to today, 14:02, before 0.1.252 changed it.")
    #expect(cost.lostSentence == "Since then: 3 jobs found, 1 card moved, 2 people added. These would be lost.")
    #expect(cost.summarize(target: version("0.1.250")) == "Going back to 0.1.250 loses 3 jobs found, 1 card moved, 2 people added.")
}

@Test func goingBackOverAMigrationWithNothingWrittenSinceLosesNothing() {
    let quiet = GoingBackReport(migration: 91, migrationReleases: [], dumps: [.init(migration: 90, dumpedAt: now, since: now, lost: [])])
    let cost = GoingBackCost.make(target: 90, database: 91, report: quiet)
    #expect(!cost.losesWrites && cost.restoresDatabase)
    #expect(cost.lostSentence == "Nothing was written since, so nothing would be lost.")
    #expect(cost.summarize(target: version("0.1.250")) == "Going back to 0.1.250 loses nothing.")
}

@Test func aCopyWhoseWritesCantBeCountedLosesAllOfThem() {
    let cost = GoingBackCost.make(target: 85, database: 91, report: report)
    #expect(cost == .uncountable(since: now.addingTimeInterval(-5 * 86400)))
    #expect(cost.losesWrites)
    #expect(cost.describeDatabase(running: version("0.1.252"), target: version("0.1.240"), now: now, calendar: utc)
        == "Going back also takes the database back to 1 Oct, 16:00, before 0.1.252 changed it.")
    #expect(cost.lostSentence == "What was written since can't be counted, and all of it would be lost.")

    // Without the hub to ask, hub-update counts once the server stopped.
    let unasked = GoingBackCost.make(target: 90, database: 91, report: nil)
    #expect(unasked == .uncountable(since: nil))
    #expect(unasked.describeDatabase(running: version("0.1.252"), target: version("0.1.250"), now: now, calendar: utc)
        == "Going back also takes the database back to before 0.1.252 changed it.")
}

@Test func withoutItsCopyTheHubCantGoBack() {
    let cost = GoingBackCost.make(target: 88, database: 91, report: report)
    #expect(cost == .noCopy)
    #expect(!cost.isPossible)
    #expect(cost.describeDatabase(running: version("0.1.252"), target: version("0.1.247"), now: now, calendar: utc)
        == "The copy of the database from before 0.1.252 changed it is gone, so the hub can't go back to 0.1.247.")
}

@Test func timesSayTheDayTheyFallOn() {
    #expect(GoingBackCost.describeTime(now.addingTimeInterval(-3600), now: now, calendar: utc) == "today, 15:00")
    #expect(GoingBackCost.describeTime(now.addingTimeInterval(-86400), now: now, calendar: utc) == "yesterday, 16:00")
    #expect(GoingBackCost.describeTime(now.addingTimeInterval(-3 * 86400), now: now, calendar: utc) == "3 Oct, 16:00")
}

@Test func theHubsReportIsReadAsTheServerWritesIt() throws {
    let json = """
    {"migration":91,"migration_releases":[{"migration":88,"release":"0.1.247"}],
     "dumps":[{"migration":88,"dumped_at":"2026-10-06T14:00:00.123456789Z","since":"2026-10-06T14:02:00Z",
               "lost":[{"rows":3,"words":"jobs found"}]},
              {"migration":85,"dumped_at":"2026-10-01T16:00:00Z","lost":[],"uncountable":"the database's statistics were reset"}]}
    """
    let read = try HubJSON.makeDecoder().decode(GoingBackReport.self, from: Data(json.utf8))
    #expect(read.migration == 91)
    #expect(read.migrationReleases == [.init(migration: 88, release: "0.1.247")])
    #expect(read.dumps.first?.lost == [LostCount(rows: 3, words: "jobs found")])
    #expect(read.dumps.first?.since == now.addingTimeInterval(-7080))
    #expect(read.dumps.last?.since == nil && read.dumps.last?.uncountable != nil)
}

@Test func anOlderReleaseKnowsTheNewestMigrationARecordedReleaseNoNewerFirstRanAt() {
    #expect(report.findMigration(of: version("0.1.247")) == 88)
    // Never installed here: it knows at least 0.1.247's, and its server
    // migrates past it from that copy if it knows more.
    #expect(report.findMigration(of: version("0.1.249")) == 88)
    #expect(report.findMigration(of: version("0.1.252")) == 91)
    #expect(report.findMigration(of: version("0.1.239")) == nil)
}

@Test func olderVersionsAreListedOnlyWhenTheyNeedNoCopyOrTheirsIsOnDisk() {
    let releases = [
        makeRelease("mac-v0.1.252"), makeRelease("mac-v0.1.251"), makeRelease("mac-v0.1.250"), makeRelease("mac-v0.1.249", prerelease: true),
        makeRelease("mac-v0.1.248", assets: false), makeRelease("mac-v0.1.247"), makeRelease("mac-v0.1.245"), makeRelease("mac-v0.1.240"),
        makeRelease("mac-v0.1.239"), makeRelease("android-v0.1.246"),
    ]
    let picked = OlderVersions.pick(releases, running: version("0.1.253"), previous: version("0.1.252"), bad: [version("0.1.251")], report: report)

    // 0.1.253 runs and 0.1.252 is the previous one, which Go back to offers;
    // 0.1.251 failed here; 0.1.249 was withdrawn; 0.1.248 has nothing to
    // download; 0.1.247 needs the copy at 88, which is gone; 0.1.239 is older
    // than any release recorded. 0.1.245 knows at least 0.1.240's 85, and
    // migrates up from that copy.
    #expect(picked.map(\.version.description) == ["0.1.250", "0.1.245", "0.1.240"])
    #expect(picked.map(\.migration) == [90, 85, 85])
    #expect(picked.first?.cost == .loses(since: now.addingTimeInterval(-7080), lost: lost))
    #expect(picked.last?.cost == .uncountable(since: now.addingTimeInterval(-5 * 86400)))
}

@Test func anOlderVersionAtTheDatabasesMigrationNeedsNoCopy() {
    let releases = [makeRelease("mac-v0.1.254"), makeRelease("mac-v0.1.253"), makeRelease("mac-v0.1.252")]
    let picked = OlderVersions.pick(releases, running: version("0.1.254"), previous: nil, bad: [], report: report)
    #expect(picked.map(\.version.description) == ["0.1.253", "0.1.252"])
    #expect(picked.allSatisfy { $0.cost == .nothingLost })
}

// MARK: The first day

private let installedAt = now.addingTimeInterval(-6 * 3600)

@Test func anUncleanExitOfAVersionInstalledTodayAsksToGoBack() {
    let run = RunRecord(version: "0.1.252", startedAt: installedAt.addingTimeInterval(-30), quitCleanly: false)
    #expect(FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: installedAt, previousRun: run, crashReports: [], now: now))
}

@Test func aCrashReportOfTheLastRunAsksToGoBack() {
    let run = RunRecord(version: "0.1.252", startedAt: now.addingTimeInterval(-3600), quitCleanly: true)
    let report = CrashReport(version: "0.1.252", crashedAt: now.addingTimeInterval(-1800))
    #expect(FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: installedAt, previousRun: run, crashReports: [report], now: now))
}

@Test func aCleanQuitOrAnOldCrashDoesntAsk() {
    let run = RunRecord(version: "0.1.252", startedAt: now.addingTimeInterval(-3600), quitCleanly: true)
    // A crash before the last run, already asked about, and another
    // version's.
    let reports = [
        CrashReport(version: "0.1.252", crashedAt: now.addingTimeInterval(-7200)), CrashReport(version: "0.1.247", crashedAt: now.addingTimeInterval(-60)),
    ]
    #expect(!FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: installedAt, previousRun: run, crashReports: reports, now: now))
}

@Test func onlyTheFirstDayOfTheVersionsOwnRunsAsks() {
    let crashed = RunRecord(version: "0.1.252", startedAt: now.addingTimeInterval(-3600), quitCleanly: false)
    // Installed more than a day ago.
    #expect(!FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: now.addingTimeInterval(-25 * 3600), previousRun: crashed, crashReports: [], now: now))
    // Never installed through hub-update.
    #expect(!FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: nil, previousRun: crashed, crashReports: [], now: now))
    // The run before was the old version's: the first launch of the new one.
    let old = RunRecord(version: "0.1.247", startedAt: now.addingTimeInterval(-3600), quitCleanly: false)
    #expect(!FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: installedAt, previousRun: old, crashReports: [], now: now))
    #expect(!FirstDayCrash.didCrash(running: version("0.1.252"), installedAt: installedAt, previousRun: nil, crashReports: [], now: now))
}

@Test func aCrashReportsHeaderNamesTheVersionAndTime() {
    let ips = """
    {"app_name":"JobSearchHub","timestamp":"2026-10-06 13:00:00.00 -0300","app_version":"0.1.252","bug_type":"309","name":"JobSearchHub"}
    {
      "uptime" : 1200
    }
    """
    #expect(CrashReport.parse(ips) == CrashReport(version: "0.1.252", crashedAt: now))
    #expect(CrashReport.parse("not json") == nil)
    #expect(CrashReport.isReportName("JobSearchHub-2026-10-06-130000.ips"))
    #expect(!CrashReport.isReportName("Finder-2026-10-06-130000.ips"))
    #expect(!CrashReport.isReportName("JobSearchHub-2026-10-06-130000.hang"))
}

@Test func theInstallTimeIsTheLastTimeTheVersionWasPutInPlace() throws {
    let json = """
    [{"from":"0.1.247","to":"0.1.252","dump":"/backups/hub-pre-migration-88.dump","at":"2026-10-05T10:00:00.5-03:00"},
     {"from":"0.1.252","to":"0.1.247","dump":"/backups/hub-pre-migration-88.dump","at":"2026-10-06T09:00:00Z","went_back":true},
     {"from":"0.1.247","to":"0.1.252","at":"2026-10-06T10:00:00Z"}]
    """
    let installs = try HubJSON.makeDecoder().decode([InstallRecord].self, from: Data(json.utf8))
    #expect(installs[1].wentBack == true)
    #expect(InstallRecord.findInstalledAt(version("0.1.252"), in: installs) == now.addingTimeInterval(-6 * 3600))
    #expect(InstallRecord.findInstalledAt(version("0.1.247"), in: installs) == now.addingTimeInterval(-7 * 3600))
    #expect(InstallRecord.findInstalledAt(version("0.1.240"), in: installs) == nil)
}

// MARK: The steps window

private func makeGoingBack(_ step: InstallStep, restored: Int = 88) -> InstallState {
    var state = InstallState.makeGoingBack(
        from: version("0.1.252"), to: version("0.1.247"), installed: URL(filePath: "/Applications/Job Search Hub.app"),
        app: URL(filePath: "/Updates/previous/Job Search Hub.app"), databaseMigration: 91, restoredMigration: restored, startedAt: now, jobPlist: nil
    )
    state.step = step
    return state
}

@Test func goingBackWritesAStateHubUpdateReadsAsGoingBack() throws {
    let state = makeGoingBack(.waitingForApp)
    let object = try JSONSerialization.jsonObject(with: state.encode()) as? [String: Any]
    #expect(object?["going_back"] as? Bool == true)
    #expect(object?["from"] as? String == "0.1.247")
    #expect(object?["to"] as? String == "0.1.252")
    #expect(object?["new_app"] as? String == "/Updates/previous/Job Search Hub.app")
    #expect(object?["from_migration"] as? Int == 88)
    #expect(object?["to_migration"] as? Int == 91)
    #expect(object?["reopen_app"] as? Bool == true)
    #expect(object?["failure"] as? String == "You went back from 0.1.252 to 0.1.247.")
    #expect(try InstallState.decode(state.encode()) == state)
}

@Test func theStepsWindowShowsGoingBack() {
    let restoring = InstallProgress.make(makeGoingBack(.restoringDatabase))
    #expect(restoring.title == "Going back to Job Search Hub 0.1.247")
    #expect(restoring.rows.map(\.title) == [
        "Server stopped", "Restoring the database from before 0.1.252", "0.1.247 back in place", "Checking 0.1.247 works", "Reopening Job Search Hub",
    ])
    #expect(restoring.rows.map(\.status) == [.done, .current, .waiting, .waiting, .waiting])

    // Without a migration since, the database stays as it is.
    let swapping = InstallProgress.make(makeGoingBack(.swappingBack, restored: 91))
    #expect(swapping.rows.map(\.title) == ["Server stopped", "0.1.247 back in place", "Checking 0.1.247 works", "Reopening Job Search Hub"])

    var back = makeGoingBack(.rolledBack)
    back.lost = "Lost: 3 jobs found."
    let done = InstallProgress.make(back)
    #expect(done.title == "Back on Job Search Hub 0.1.247")
    #expect(done.subtitle == "Lost: 3 jobs found.")
    #expect(done.fraction == 1)
    #expect(InstallOutcome.make(back) == .rolledBack(message: "You went back from 0.1.252 to 0.1.247.", lost: "Lost: 3 jobs found."))

    var stayed = makeGoingBack(.abandoned)
    stayed.failure = "The server didn't stop, so the hub stayed on 0.1.252."
    #expect(InstallProgress.make(stayed).title == "Stayed on Job Search Hub 0.1.252")
}
