import Foundation
@testable import JobSearchHubCore
import Testing

private func makeCV(label: String, summary: String, work: [[String]], citations: [String: String] = [:]) -> CV {
    CV(id: UUID(), kind: "tailored", content: CVContent(
        basics: CVBasics(name: "Ada", label: label, summary: summary),
        work: work.enumerated().map { index, highlights in CVWork(name: "Co\(index)", position: "Engineer", startDate: "2020-01", endDate: nil, highlights: highlights) }
    ), citations: citations, hasPDF: false)
}

@Test func aTailoredCVShowsWhatItKeptRewordedAddedAndLeftOut() {
    let base = makeCV(label: "Engineer", summary: "Builds.", work: [["Built the API.", "Led the web app."], ["Wrote reports."]])
    let tailored = makeCV(label: "Front-End Engineer", summary: "Builds.", work: [["Led the React web app.", "Shipped the design system."], ["Wrote reports."]],
                          citations: ["w0h0": "base:w0h1", "w0h1": "entry:abc", "w1h0": "base:w1h0"])

    let comparison = CVComparison(tailored: tailored, base: base)

    #expect(comparison.isLabelChanged && !comparison.isSummaryChanged)
    #expect(comparison.roles[0].bullets.map(\.status) == [.reworded(from: "Led the web app."), .fromEntry])
    #expect(comparison.roles[0].leftOut == ["Built the API."])
    #expect(comparison.roles[1].bullets.map(\.status) == [.kept] && comparison.roles[1].leftOut.isEmpty)
}

@Test func aCVDecodesWithItsCitationsAndPDF() throws {
    let json = #"{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","kind":"tailored","has_pdf":true,"pdf_path":"/Users/ada/Interview/CVs/Acme - Engineer/Ada_CV.pdf","citations":{"w0h0":"base:w1h0"},"#
        + #""content":{"basics":{"name":"Ada","label":"L","summary":"S","location":{"city":"X"}},"work":[{"name":"Co","position":"P","startDate":"2020-01","highlights":["H"]}]}}"#
    let cv = try HubJSON.makeDecoder().decode(CV.self, from: Data(json.utf8))
    #expect(cv.hasPDF && cv.citations["w0h0"] == "base:w1h0" && cv.content.work.first?.startDate == "2020-01")
    #expect(cv.printedFile?.path == "/Users/ada/Interview/CVs/Acme - Engineer/Ada_CV.pdf")
    var edited = cv
    edited.hasPDF = false
    #expect(edited.printedFile == nil)
}

@Test func aRecruiterScreenDecodesAndKnowsWhenItsCVChanged() throws {
    let json = #"{"job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","cv_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","cv_updated_at":"2026-09-30T22:00:00Z","model":"local","created_at":"2026-09-30T22:01:00Z","#
        + #""screen":{"issues":[{"issue":"No GraphQL","posting_says":"GraphQL APIs","response":"The CV can't answer it."}],"summary":"Close.","verdict":"likely_reject"}}"#
    let screen = try HubJSON.makeDecoder().decode(CVScreen.self, from: Data(json.utf8))
    #expect(screen.screen.verdict == .likelyReject && screen.screen.issues.first?.postingSays == "GraphQL APIs")
    var cv = CV(id: UUID(), kind: "tailored", content: CVContent(basics: CVBasics(name: "Ada", label: "L", summary: "S"), work: []), citations: [:], hasPDF: false)
    cv.updatedAt = screen.cvUpdatedAt
    #expect(!screen.isOutdated(for: cv))
    cv.updatedAt = screen.cvUpdatedAt.addingTimeInterval(60)
    #expect(screen.isOutdated(for: cv))
}
