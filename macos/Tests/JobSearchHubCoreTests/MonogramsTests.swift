@testable import JobSearchHubCore
import Testing

@Test func aMonogramShowsTheFirstLettersOfUpToTwoWords() {
    #expect(Monograms.getLetters(for: "Northwind") == "N")
    #expect(Monograms.getLetters(for: "northwind") == "N")
    #expect(Monograms.getLetters(for: "Acme Labs") == "AL")
    #expect(Monograms.getLetters(for: "riley  chen example") == "RC")
    #expect(Monograms.getLetters(for: "  (acme)") == "A")
    #expect(Monograms.getLetters(for: "37signals") == "S")
    #expect(Monograms.getLetters(for: "Studio 54") == "S5")
    #expect(Monograms.getLetters(for: "") == "?")
    #expect(Monograms.getLetters(for: " - ") == "?")
}

@Test func aMonogramKeepsItsHueForTheSameName() {
    #expect(Monograms.getHueIndex(for: "Northwind", count: 6) == Monograms.getHueIndex(for: "northwind", count: 6))
    #expect((0..<6).contains(Monograms.getHueIndex(for: "Globex", count: 6)))
    #expect(Monograms.getHueIndex(for: "Globex", count: 0) == 0)
}
