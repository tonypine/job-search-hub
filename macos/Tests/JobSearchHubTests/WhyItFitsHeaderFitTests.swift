import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// The Why it fits card on a job page's rail, measured offscreen in a hosting
// view that no window holds, as PipelineHeaderFitTests does. TP-659's epic
// review found its meta cut short beside Write full brief on the 340 pt rail
// ("local model · 5 minutes a…", TP-1129); this checks that the meta shows
// whole at any age a brief can have. The model select beside Write full
// brief (TP-1267) left no room for both in the header, so the run control
// is on a line of its own at the card's foot, which fits the rail too.

/// Ages from seconds to most of a year, with the widest of each unit.
private let ages: [TimeInterval] = [30, 5 * 60, 59 * 60, 23 * 3600, 3 * 86400, 20 * 86400, 300 * 86400]

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

/// The full brief's kind, routed to Claude Sonnet, as a job's runnable
/// kinds list it.
private func makeFullBrief() throws -> RunnableKind {
    let json = #"""
    {"id":"job_brief_full","title":"Full briefs","description":"Claude writes a full brief.","subject_type":"job","engine":"claude_one_shot",
     "automatic":true,"paused":false,"route":{"provider_id":"dddddddd-0000-4000-8000-000000000001","provider":"Claude","model":"sonnet"},
     "can_run":true,"needs_model":false,"needs_note":false,"running_starts":[]}
    """#
    return try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(json.utf8))
}

/// The card as the rail shows a pre-brief: the title with who wrote it and
/// when, then at its foot the run control, the model select and ▶ Write
/// full brief.
@MainActor
private func makeCard(writtenAgo age: TimeInterval) throws -> some View {
    let meta = JobDetailView.describeAuthor(isFull: false, writtenAt: Date.now.addingTimeInterval(-age))
    let client = HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test")
    let selection = RunModelSelection(kind: try makeFullBrief())
    return HubCard("Why it fits", meta: meta) {
        JobSectionRunControl(kind: JobRuns.fullBriefKind, jobID: UUID(), client: client, selection: selection) { _ in
            JobDetailView.makeFullBriefButton(isBusy: false) {}
        }
    }
}

@MainActor
@Test(arguments: ages)
func theBriefsMetaAndItsRunControlFitTheRail(age: TimeInterval) throws {
    let width = measureWidth(try makeCard(writtenAgo: age))
    let rail = JobPostingTab<EmptyView>.railWidth
    // The select and the button alone are wider than this; less means the
    // card wasn't measured.
    #expect(width > 150)
    #expect(width <= rail, "The card needs \(width) pt; the rail is \(rail) pt")
}
