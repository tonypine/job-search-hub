import AppKit
import Foundation
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// A row of a job's Run popover, measured offscreen in a hosting view that no
// window holds, as SecondRouteRowFitTests does. QA found every row's second
// line cut to "No model is routed for it yet · kind pau…" in the 420 pt
// popover beside the 150 pt model column (TP-1064), so the "kind paused" the
// ticket asks for couldn't be read. These check the row against the room the
// popover gives it: one line for each reason the hub gives and for a paused
// kind's invalid run, and a second line, not a cut one, past that.

/// Why the hub can't run a kind yet (server/internal/modelwork/starts.go).
private let hubReasons = [
    "no model is routed for it yet",
    "needs the posting's text first",
    "needs the job's facts first",
    "needs your base CV first",
    "needs the tailored CV first",
    "once you pursue the job",
]

/// A paused kind for a made-up job: one that can't run for `reason`, or,
/// without one, a recruiter screen whose last run, today, gave an invalid
/// answer.
private func makeKind(reason: String?) throws -> RunnableKind {
    let state = reason.map { #""can_run":false,"reason":"\#($0)""# } ?? #"""
    "can_run":true,"last_run":{"id":"cccccccc-0000-4000-8000-000000000002","record":"model_call","kind":"recruiter_screen",
     "subject":{"type":"job","id":"cccccccc-0000-4000-8000-000000000001","title":"Staff engineer"},"model":"qwen3-8b.gguf","provider":"This Mac",
     "started_at":"\#(Date.now.ISO8601Format())","outcome":"invalid","prompt_tokens":0,"completion_tokens":0,"started_by_owner":false}
    """#
    let json = #"""
    {"id":"recruiter_screen","title":"Recruiter screens","description":"Reads the CV as a recruiter.","subject_type":"job","engine":"model_call",
     "automatic":true,"paused":true,\#(state),"needs_note":false,"running_starts":[]}
    """#
    return try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(json.utf8))
}

@MainActor
private func makeRow(_ kind: RunnableKind) -> JobRunRow {
    JobRunRow(kind: kind, model: nil, running: nil, isBusy: false) {}
}

@MainActor
private func measure(_ view: some View) -> CGSize {
    NSHostingView(rootView: view).fittingSize
}

@MainActor
@Test func aPausedKindsSecondLineFitsOneLineOfThePopover() throws {
    let room = JobRunPopover.rowWidth
    for kind in try hubReasons.map(makeKind(reason:)) + [makeKind(reason: nil)] {
        let line = JobRuns.describeState(of: kind, now: .now)
        #expect(line.hasSuffix(" · kind paused"))
        let lineWidth = measure(Text(line).font(.hubCaption).fixedSize()).width
        let rowWidth = measure(makeRow(kind).fixedSize()).width
        // Less than the line means nothing was measured.
        #expect(rowWidth > lineWidth + JobRunRow.modelWidth)
        #expect(rowWidth <= room, "\"\(line)\" needs a \(rowWidth) pt row; the popover gives it \(room) pt")
    }
}

@MainActor
@Test func aSecondLineTooLongForOneLineWrapsRatherThanBeingCut() throws {
    let room = JobRunPopover.rowWidth
    let row = makeRow(try makeKind(reason: "needs the job's facts and a tailored CV for this posting first"))
    let oneLine = measure(row.fixedSize())
    #expect(oneLine.width > room, "The line fits one line of \(room) pt, so this checks nothing")
    // A cut line would keep the row as tall as one whose line fits.
    let atRoom = measure(row.frame(width: room))
    let fitting = measure(makeRow(try makeKind(reason: "once you pursue the job")).frame(width: room))
    let lineHeight = measure(Text("Never").font(.hubCaption).fixedSize()).height
    #expect(atRoom.height >= fitting.height + lineHeight * 0.9, "At \(room) pt the row is \(atRoom.height) pt tall; with one line it is \(fitting.height) pt")
}
