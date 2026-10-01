import Foundation

/// A technology good fits keep asking for that the knowledge base never
/// names, and the plan to close it: learn it, or show it in a portfolio piece.
public struct MarketGap: Decodable, Equatable, Identifiable, Sendable {
    public var technology: String
    public var jobCount: Int
    public var goodFits: Int
    public var planKind: String
    public var plan: String
    public var computedAt: Date

    public var id: String { technology }

    /// "Asked for by 14 of 43 good fits".
    public var demandText: String {
        "Asked for by \(jobCount) of \(goodFits) good fits"
    }

    /// "Learn" or "Show it", the plan's kind as the page names it.
    public var planTitle: String {
        switch planKind {
        case "learn": "Learn"
        case "portfolio": "Show it"
        default: "Plan"
        }
    }
}

public struct MarketGapsResponse: Decodable, Sendable {
    public var gaps: [MarketGap]
}

public extension HubClient {
    func getMarketGaps() async throws -> [MarketGap] {
        try await get("v1/market-gaps", as: MarketGapsResponse.self).gaps
    }
}
