import AppKit
@testable import JobSearchHub
@testable import JobSearchHubCore
import SwiftUI
import Testing

// The Companies table beside the inspector, its cells measured offscreen in
// a hosting view that no window holds. In a 1400 pt window with the
// inspector at 420 pt, TP-659's epic review found names and their context
// cut ("Orchard Mesa Ana…", "Developer tools · 51-2…") and the header
// "People you kn…", while Open jobs, You and Board kept wide columns
// (TP-1131). These check the Company column's room against what the seeded
// hub's rows need, and the fixed columns' against their headers and values.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset, and the inspector at its ideal width.
private let contentColumn: CGFloat = 1400 - 248 - InspectorWidth.ideal

/// What the table takes besides its columns' widths: 10 pt at each end, and
/// its cell spacing beside each of the five columns.
private let tableChrome: CGFloat = 2 * 10 + 5 * CompanyRowLayout.cellSpacing

/// The Company column's width, the room the fixed columns leave it.
private let companyColumn = contentColumn - tableChrome - CompanyRowLayout.fixedColumnsWidth

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

private func makeSummary(
    _ name: String, industry: String, size: String, phase: String? = nil, fittingJobs: Int = 1, bestMatch: JobMatch? = .strong,
    unseenUpdates: Int = 0
) -> CompanySummary {
    let seen = Date(timeIntervalSince1970: 0)
    let company = Company(
        id: UUID(), name: name, domain: name.lowercased().filter(\.isLetter) + ".example", employeeCountRange: size, industry: industry,
        createdAt: seen, updatedAt: seen
    )
    return CompanySummary(
        company: company, watchedSince: seen, jobBoards: [], peopleCount: 0, connectionCount: 0, knownPeople: [], applicationPhase: phase,
        fittingJobs: fittingJobs, bestMatch: bestMatch, newestFittingJobSeenAt: nil, unseenUpdates: unseenUpdates
    )
}

/// The seeded hub's companies (server/scripts/dev-server-seed), the longest
/// names with their unseen dot and the longest context lines.
private func makeWidestRows() -> [CompanySummary] {
    [
        makeSummary("Orchard Mesa Analytics", industry: "Analytics", size: "501-1,000", phase: "Interviewing", unseenUpdates: 1),
        makeSummary("Quillfeather Systems", industry: "Logistics software", size: "201-500", phase: "Applied", bestMatch: .possible, unseenUpdates: 1),
        makeSummary("Larkspur Freight", industry: "Freight marketplace", size: "11-50", phase: "Applied", unseenUpdates: 1),
        makeSummary("Brightwater Labs", industry: "Developer tools", size: "51-200", phase: "Applied"),
        makeSummary("Copperleaf Health", industry: "Digital health", size: "201-500", phase: "Saved", fittingJobs: 2),
    ]
}

@MainActor
private func makeCell(_ summary: CompanySummary) -> CompanyCell {
    CompanyCell(row: .company(summary), researchStep: nil)
}

@MainActor
@Test func theCompanyColumnGetsTheRoomTheInspectorLeaves() {
    // The table's default widths fit it beside the inspector.
    #expect(CompanyRowLayout.companyIdealWidth <= companyColumn, "The Company column has \(companyColumn) pt")
    #expect(CompanyRowLayout.companyMinWidth < CompanyRowLayout.companyIdealWidth)
    // "Logistics software · 201-500 people" takes about 204 pt.
    let textRoom = companyColumn - measureWidth(Monogram("Quillfeather Systems")) - Space.m
    #expect(textRoom >= 204, "The Company column leaves \(textRoom) pt for the name and its context")
}

@MainActor
@Test func namesAndTheirContextShowInFull() {
    for summary in makeWidestRows() {
        let width = measureWidth(makeCell(summary))
        // The context alone is this wide; less means nothing was measured.
        let context = measureWidth(Text(summary.contextLine).font(.hubSecondary))
        #expect(width > context)
        #expect(width <= companyColumn, "\(summary.company.name) needs \(width) pt; the Company column is \(companyColumn) pt")
    }
}

@MainActor
@Test func theFixedColumnsHoldTheirWidestValues() {
    let openJobs = measureWidth(OpenJobsCell(fittingJobs: 2, bestMatch: .possible))
    #expect(openJobs > 100)
    #expect(openJobs <= CompanyRowLayout.openJobsWidth, "\"2 open · Best: Possible\" needs \(openJobs) pt")
    let interviewing = measureWidth(StandingCell(standing: .phase("Interviewing")))
    let newJobToday = measureWidth(StandingCell(standing: .newJobToday))
    #expect(interviewing > 60 && newJobToday > 60)
    #expect(max(interviewing, newJobToday) <= CompanyRowLayout.youWidth, "The You column's values need \(interviewing) and \(newJobToday) pt")
    let faces = measureWidth(KnownPeopleCell(names: ["Ada Example", "Kim Ray Example", "Jo Lee Example"], count: 12))
    #expect(faces > 40)
    #expect(faces <= CompanyRowLayout.peopleWidth, "Three faces and a count need \(faces) pt")
}

/// AppKit's header margins, as measured on QA's screenshot of the Jobs table
/// at AppKit's own 17 pt spacing: 10 pt before a header's title and 11 pt
/// after it (JobsTableFitTests).
@Test func theFixedColumnsLeaveTheirHeadersRoom() {
    // The headers' 11 pt system font.
    let font = NSFont.systemFont(ofSize: NSFont.smallSystemFontSize)
    func measureHeader(_ title: String) -> CGFloat {
        (title as NSString).size(withAttributes: [.font: font]).width
    }
    let columns: [(title: String, width: CGFloat)] = [
        ("Open jobs", CompanyRowLayout.openJobsWidth),
        ("People you know", CompanyRowLayout.peopleWidth),
        ("You", CompanyRowLayout.youWidth),
        ("Board", CompanyRowLayout.boardWidth),
    ]
    for column in columns {
        let needed = 10 + measureHeader(column.title) + 11
        #expect(needed > 30, "Nothing was measured for \(column.title)")
        #expect(column.width + CompanyRowLayout.cellSpacing >= needed, "\(column.title)'s header needs \(needed) pt")
    }
}

/// The widest rows drawn offscreen at the Company column's width, in both
/// appearances. With HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the
/// PNGs are written there.
@MainActor
@Test func theWidestRowsDrawAtTheCompanyColumnsWidth() throws {
    let rows = makeWidestRows()
    for colorScheme in [ColorScheme.light, .dark] {
        let content = VStack(spacing: Space.s) {
            ForEach(rows) { summary in makeCell(summary) }
        }
        .frame(width: companyColumn)
        .padding(Space.s)
        .background(Color(nsColor: .windowBackgroundColor))
        .tint(.hubAccent)
        .environment(\.colorScheme, colorScheme)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        let image = try #require(renderer.cgImage)
        #expect(image.width == Int((companyColumn + 2 * Space.s) * 2))
        try writeSnapshot(image, named: colorScheme == .light ? "light" : "dark")
    }
}

private func writeSnapshot(_ image: CGImage, named name: String) throws {
    guard let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
          let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])
    else { return }
    let url = URL(filePath: folder)
    try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    try png.write(to: url.appending(path: "companies-widest-rows-\(name).png"))
}
