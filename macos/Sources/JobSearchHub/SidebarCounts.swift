import JobSearchHubCore
import SwiftUI

/// The sidebar's badges besides Today's unseen updates: the jobs waiting in
/// the Decide queue, and the Pipeline's follow-ups due today or overdue. A
/// failed read keeps the last counts.
@MainActor
@Observable
final class SidebarCounts {
    private(set) var toDecide = 0
    private(set) var followUpsDue = 0

    func refresh(with client: HubClient) async {
        async let queue = try? client.getDecisionQueue()
        async let pipeline = try? client.getPipeline()
        if let queue = await queue {
            toDecide = queue.items.count
        }
        if let pipeline = await pipeline {
            followUpsDue = PipelineBoard(pipeline).getDueCount(now: .now)
        }
    }

    func getCount(for page: Page, unseen: Int) -> Int {
        switch page {
        case .decide: toDecide
        case .pipeline: followUpsDue
        case .today: unseen
        default: 0
        }
    }
}
