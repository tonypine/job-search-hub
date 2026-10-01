import JobSearchHubCore
import SwiftUI

/// The skills good fits keep asking for that the knowledge base never names,
/// with how many ask and the plan to close each. The hub refreshes them daily.
struct MarketGapsSection: View {
    let client: HubClient
    @State private var gaps: [MarketGap] = []
    @State private var errorMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Market gaps").font(.title3.weight(.semibold))
            Text("Technologies good fits keep asking for that your knowledge base never names. A skill you have but never wrote down closes by adding it there.")
                .font(.callout).foregroundStyle(.secondary)
            if let errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            } else if gaps.isEmpty {
                Text("The hub lists the gaps once a day, from the good fits' facts.").foregroundStyle(.secondary)
            } else {
                ForEach(gaps) { gap in
                    VStack(alignment: .leading, spacing: 3) {
                        HStack(alignment: .firstTextBaseline) {
                            Text(gap.technology).fontWeight(.semibold)
                            Text(gap.demandText).font(.caption).foregroundStyle(.secondary)
                        }
                        if !gap.plan.isEmpty {
                            (Text("\(gap.planTitle): ").fontWeight(.medium) + Text(gap.plan)).font(.callout)
                        }
                    }
                }
                if let computedAt = gaps.first?.computedAt {
                    Text("Listed \(computedAt.formatted(.relative(presentation: .named)))").font(.caption).foregroundStyle(.secondary)
                }
            }
        }
        .task {
            do {
                gaps = try await client.getMarketGaps()
                errorMessage = nil
            } catch {
                errorMessage = "Could not load the market gaps: \(error)"
            }
        }
    }
}
