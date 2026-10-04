import JobSearchHubCore
import SwiftUI

/// The skills the jobs that pass the screen keep asking for that the
/// knowledge base never names, with how many ask and the plan to close each. The hub refreshes them daily.
struct MarketGapsSection: View {
    let client: HubClient
    @State private var gaps: [MarketGap] = []
    @State private var failure: HubFailure?

    var body: some View {
        HubSection("Market gaps") {
            Text("Technologies the jobs that pass the screen keep asking for that your knowledge base never names. A skill you have but never wrote down closes by adding it there.")
                .font(.hubSecondary).foregroundStyle(.secondary)
            if let failure {
                HubErrorView(failure)
            } else if gaps.isEmpty {
                Text("The hub lists the gaps once a day, from the facts of the jobs that pass the screen.").foregroundStyle(.secondary)
            } else {
                ForEach(gaps) { gap in
                    VStack(alignment: .leading, spacing: Space.xs) {
                        HStack(alignment: .firstTextBaseline) {
                            Text(gap.technology).fontWeight(.semibold)
                            Text(gap.demandText).font(.hubCaption).foregroundStyle(.secondary)
                        }
                        if !gap.plan.isEmpty {
                            Text("\(Text("\(gap.planTitle): ").fontWeight(.medium))\(gap.plan)").font(.hubSecondary)
                        }
                    }
                }
                if let computedAt = gaps.first?.computedAt {
                    Text("Listed \(computedAt.formatted(.relative(presentation: .named)))").font(.hubCaption).foregroundStyle(.secondary)
                }
            }
        }
        .task {
            do {
                gaps = try await client.getMarketGaps()
                failure = nil
            } catch {
                failure = HubFailure("Couldn't load the market gaps", error)
            }
        }
    }
}
