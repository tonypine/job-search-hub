import AppKit
import Foundation
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// The Posting tab on a job's page, laid out offscreen in a hosting view that
// no window holds, as PipelineHeaderFitTests does. TP-659's epic review found
// Decide's page (the narrow layout, in an 851 pt column) drawing its scroller
// in the middle of the page, over the key facts and the posting (TP-1146);
// the wide layout's scroller covered the rail's cards (TP-1129). The design
// draws none, so neither layout's scroll view has a vertical scroller.

private let detailsJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"job_board","title":"Engineering Manager, iOS",
        "url":"https://example.com/jobs/7c9e","description":"## The role\n\nYou'll work with Swift.\n\n## What we're looking for\n\n- Swift experience\n- Care for performance",
        "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "company_name":"Example Labs",
 "fit":{"level":"unclear","checks":[]},
 "unseen_updates":0}
"""#

private func findScrollViews(in view: NSView) -> [NSScrollView] {
    (view as? NSScrollView).map { [$0] } ?? view.subviews.flatMap(findScrollViews)
}

/// The Posting tab in the layout given, at Decide's column size.
@MainActor
private func layOutPostingTab(_ layout: JobPostingLayout) throws -> [NSScrollView] {
    let details = try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(detailsJSON.utf8))
    let client = HubClient(baseURL: try #require(URL(string: "http://127.0.0.1:1")), token: "")
    let tab = JobPostingTab(
        jobID: details.job.id, details: details, client: client, model: JobDetailModel(), postingAnchor: "posting", layout: layout,
        scrollTarget: .constant(nil)
    ) {
        Text("Why it fits")
    }
    let host = NSHostingView(rootView: tab.frame(width: 851, height: 629))
    host.frame = NSRect(x: 0, y: 0, width: 851, height: 629)
    host.layoutSubtreeIfNeeded()
    return findScrollViews(in: host)
}

@MainActor
@Test(arguments: [JobPostingLayout.narrow, .wide(showsOutline: false), .wide(showsOutline: true)])
func aJobsPageDrawsNoVerticalScroller(layout: JobPostingLayout) throws {
    let scrollViews = try layOutPostingTab(layout)
    // Less means the page wasn't laid out, and there's nothing to check.
    #expect(!scrollViews.isEmpty)
    for scrollView in scrollViews {
        #expect(!scrollView.hasVerticalScroller, "A scroll view \(scrollView.frame.width) pt wide has a vertical scroller")
    }
}
