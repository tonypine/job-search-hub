import Foundation
@testable import JobSearchHubCore
import Testing

// Each test is one row of the state-to-tone table in docs/design/ui-redesign.md.

@Test func matchesReadStrongPossibleStretchMismatch() {
    #expect([JobMatch.strong, .possible, .stretch, .mismatch].map(\.tone) == [.positive, .accent, .caution, .neutral])
}

@Test func theScreenPassesIsUnclearOrFails() {
    #expect([FitLevel.good, .unclear, .poor].map(\.tone) == [.positive, .caution, .negative])
    #expect([FitVerdict.yes, .unclear, .no].map(\.tone) == [.positive, .caution, .negative])
}

@Test func theScreenReadsPassesUnclearOrFails() {
    #expect([FitLevel.good, .unclear, .poor].map(\.title) == ["Passes", "Unclear", "Fails"])
    #expect([FitLevel.good, .unclear, .poor].map(\.label) == ["Passes screen", "Screen unclear", "Fails screen"])
}

@Test func eachScreenVerdictHasItsOwnSymbol() {
    let symbols = [FitVerdict.yes, .unclear, .no].map(\.symbolName)
    #expect(Set(symbols).count == 3)
}

@Test func followUpsAreOverdueDueTodayOrLater() {
    #expect(FollowUpStatus.overdue(days: 2).tone == .negative)
    #expect(FollowUpStatus.dueToday.tone == .caution)
    #expect(FollowUpStatus.dueIn(days: 3).tone == .neutral)
}

@Test func whatIsSetAsideIsNeutralWithItsSymbol() {
    #expect(SetAside.allCases.allSatisfy { $0.tone == .neutral })
    #expect(SetAside.allCases.allSatisfy { !$0.symbolName.isEmpty })
    #expect(SetAside.closed.symbolName != SetAside.skipped.symbolName)
}

@Test func sessionsWorkWaitOrIdle() {
    #expect([SessionActivity.working, .blocked, .idle].map(\.tone) == [.accent, .caution, .positive])
}

@Test func cvScreensPassAreBorderlineOrAreRejected() {
    #expect([CVScreenVerdict.likelyPass, .borderline, .likelyReject].map(\.tone) == [.positive, .caution, .negative])
}

@Test func runOutcomesKeepFailuresNegative() {
    #expect(Tone.ofRunOutcome("succeeded") == .positive)
    #expect(Tone.ofRunOutcome("running") == .accent)
    #expect(Tone.ofRunOutcome("invalid") == .caution)
    #expect(Tone.ofRunOutcome("failed") == .negative)
    #expect(Tone.ofRunOutcome("anything new") == .negative)
}

@Test func connectionProblemsAreCautionAndRefusalsNegative() {
    #expect(ConnectionStatus.connected.tone == .positive)
    #expect(ConnectionStatus.unchecked.tone == .neutral)
    #expect(ConnectionStatus.serverUnreachable("down").tone == .caution)
    #expect(ConnectionStatus.tokenRefused.tone == .negative)
    #expect(ConnectionStatus.upgradeRequired("Update the app.").tone == .negative)
}

@Test func theServerRunsWaitsOrIsStopped() {
    #expect(ServerLaunchAgent.State.running(pid: 1).tone == .positive)
    #expect(ServerLaunchAgent.State.waiting(lastExitCode: nil).tone == .caution)
    #expect(ServerLaunchAgent.State.stopped.tone == .neutral)
}

@Test func comparisonVerdictsAreRightOrWrong() {
    #expect(ComparisonVerdictKind.right.tone == .positive)
    #expect(ComparisonVerdictKind.wrong.tone == .negative)
}
