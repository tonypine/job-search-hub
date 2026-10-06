import Foundation
@testable import JobSearchHubCore
import Testing

private let now = Date(timeIntervalSince1970: 1_791_300_000)
private let running = HubVersion("0.1.247")!
private let ready = ReadyVersion(
    version: HubVersion("0.1.252")!,
    whatsNew: WhatsNew(releaseCount: 1, notes: [ChangeNote(kind: .new, parts: [.mac], text: "Suggest a second route", pullRequest: 67)]),
    changesDatabase: true
)

@Test func aCheckUnderWaySaysSoFirst() {
    let facts = NewVersionFacts(running: running, isChecking: true, ready: ready, isRunningWithdrawn: true)
    #expect(NewVersionStatus.make(facts, now: now) == .checking)
}

@Test func withoutANewerReleaseTheAppIsUpToDate() {
    let facts = NewVersionFacts(running: running, lastCheckedAt: now, newestRelease: running)
    #expect(NewVersionStatus.make(facts, now: now) == .upToDate(newest: running, checkedAt: now))
}

@Test func aReadyVersionIsShownInFull() {
    #expect(NewVersionStatus.make(NewVersionFacts(running: running, lastCheckedAt: now, ready: ready), now: now) == .ready)
}

@Test func aLocalBuildSaysSoEvenWithAVersionReady() {
    let facts = NewVersionFacts(running: HubVersion("0.1.0-dev.abc1234")!, lastCheckedAt: now, ready: ready)
    #expect(NewVersionStatus.make(facts, now: now) == .localBuild(commit: "abc1234"))
}

@Test func aNewerDownloadThatFailedItsChecksIsNamed() {
    let failed = FailedVersion(version: HubVersion("0.1.253")!, problem: .otherTeam(found: "OTHERTEAM9", expected: "OWNERTEAM1"))
    #expect(NewVersionStatus.make(NewVersionFacts(running: running, ready: ready, failed: failed), now: now) == .failedChecks(failed))
    #expect(failed.title == "0.1.253 didn't pass its checks")

    // One older than the version ready is history.
    let older = FailedVersion(version: HubVersion("0.1.250")!, problem: .checksumMismatch(expected: "a", found: "b"))
    #expect(NewVersionStatus.make(NewVersionFacts(running: running, ready: ready, failed: older), now: now) == .ready)
}

@Test func checksFailingForADaySaySo() {
    let since = now.addingTimeInterval(-NewVersionStatus.staleAfter)
    #expect(NewVersionStatus.make(NewVersionFacts(running: running, failingSince: since), now: now) == .cannotCheck(since: since))

    let recent = now.addingTimeInterval(-3600)
    #expect(NewVersionStatus.make(NewVersionFacts(running: running, lastCheckedAt: recent, failingSince: recent), now: now)
        == .upToDate(newest: nil, checkedAt: recent))
}

@Test func aWithdrawnRunningVersionSaysSo() {
    let facts = NewVersionFacts(running: running, lastCheckedAt: now, ready: ready, isRunningWithdrawn: true)
    #expect(NewVersionStatus.make(facts, now: now) == .withdrawn(running))
}

@Test func theInstallLineSaysWhatTheInstallWillDo() {
    #expect(ready.installSummary == "The server restarts for about 10 seconds. This version changes the database, so a copy is saved first.")

    var phone = ready
    phone.changesDatabase = false
    phone.whatsNew.notes.append(ChangeNote(kind: .new, parts: [.phone], text: "Snooze a follow-up", pullRequest: 73))
    #expect(phone.installSummary == "The server restarts for about 10 seconds. The database stays as it is. The phone changes install on the phone.")
}
