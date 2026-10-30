@testable import JobSearchHubCore
import Testing

@Test func aChartWhoseBusiestBucketHoldsOneRunReadsZeroAndOne() {
    // The epic's Find jobs page: Swift Charts' own pick was 0, 0.5 and 1,
    // and 0.5 read "0".
    #expect(RunValueAxis.getTicks(max: 1) == [0, 1])
}

@Test func anEmptyChartReadsZeroAndOne() {
    #expect(RunValueAxis.getTicks(max: 0) == [0, 1])
}

@Test func aFewRunsStepByOne() {
    #expect(RunValueAxis.getTicks(max: 2) == [0, 1, 2])
    #expect(RunValueAxis.getTicks(max: 3) == [0, 1, 2, 3])
}

@Test func moreRunsStepByTwoOrFive() {
    #expect(RunValueAxis.getTicks(max: 4) == [0, 2, 4])
    #expect(RunValueAxis.getTicks(max: 5) == [0, 2, 4, 6])
    #expect(RunValueAxis.getTicks(max: 7) == [0, 5, 10])
    #expect(RunValueAxis.getTicks(max: 15) == [0, 5, 10, 15])
    #expect(RunValueAxis.getTicks(max: 16) == [0, 10, 20])
}

@Test func tokensStepByAPowerOfTen() {
    #expect(RunValueAxis.getTicks(max: 1_234_567) == [0, 500_000, 1_000_000, 1_500_000])
    #expect(RunValueAxis.getTicks(max: 2_500) == [0, 1_000, 2_000, 3_000])
}

@Test func everyGridlineIsAWholeStepTheLastAtOrAboveTheHighestBar() {
    for max in 0 ... 5_000 {
        let ticks = RunValueAxis.getTicks(max: max)
        #expect(ticks.first == 0)
        #expect(ticks.count >= 2 && ticks.count <= RunValueAxis.maxSteps + 1)
        #expect(ticks.last! >= max)
        let step = ticks[1]
        #expect(zip(ticks, ticks.dropFirst()).allSatisfy { $1 - $0 == step })
    }
}

@Test func noTwoGridlinesReadTheSame() {
    for max in [1, 3, 9, 40, 999, 1_001, 9_999, 12_345, 99_999, 1_000_001, 12_345_678] {
        let labels = RunValueAxis.getTicks(max: max).map { RunWindowFormat.formatCompact(Double($0)) }
        #expect(Set(labels).count == labels.count, "max \(max): \(labels)")
    }
}
