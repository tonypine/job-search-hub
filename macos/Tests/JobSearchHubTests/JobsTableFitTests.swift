import AppKit
@testable import JobSearchHub
@testable import JobSearchHubCore
import SwiftUI
import Testing

// The Jobs table beside the inspector, its job cells measured offscreen in a
// hosting view that no window holds. In a 1400 pt window with the inspector
// at 480 pt, TP-659's epic review found every title cut ("Senior iOS Eng…"),
// the Posted header cut to "Pos…", and the selected row's title drawn as "…"
// beside its actions and its "In Saved" chip (TP-1123). With the inspector at
// 420 pt, QA still found titles with a chip cut, and the selected row's
// title and its company and location: AppKit's 17 pt between columns took
// the room the design gives the titles. The next review found a hovered
// row's "In Applied" chip drawn as its symbol alone, even at full width
// (TP-1135): chips keep their words beside the actions. These check the Job
// column's room against what the seeded hub's rows need.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset, and the inspector at its ideal width.
private let contentColumn: CGFloat = 1400 - 248 - InspectorWidth.ideal

/// What the table takes besides its columns' widths, as measured on QA's
/// screenshot: 10 pt at each end, and its cell spacing beside each of the
/// five columns.
private let tableChrome: CGFloat = 2 * 10 + 5 * JobRowLayout.cellSpacing

/// The Job column's width, the room the fixed columns leave it.
private let jobColumn = contentColumn - tableChrome - JobRowLayout.fixedColumnsWidth

/// The Job column in a 1400 pt window with the inspector closed.
private let fullWidthJobColumn = 1400 - 248 - tableChrome - JobRowLayout.fixedColumnsWidth

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

private func makeItem(
    _ title: String, company: String?, location: String?, phase: String? = nil, unseenUpdates: Int = 0
) -> JobListItem {
    let seen = Date(timeIntervalSince1970: 0)
    let job = Job(id: UUID(), source: "manual", title: title, location: location, url: "https://example.com/job", firstSeenAt: seen, lastSeenAt: seen)
    var item = JobListItem(job: job, companyName: company, fit: JobFit(level: .good, checks: []), unseenUpdates: unseenUpdates)
    item.pipelinePhase = phase
    return item
}

@MainActor
private func makeCell(_ item: JobListItem, isSelected: Bool, isHovered: Bool = false) -> JobCell {
    let hover = JobRowHover()
    if isHovered { hover.id = item.id }
    return JobCell(item: item, isSelected: isSelected, hover: hover, isSkipped: false, onDecide: { _ in }, onRestore: {})
}

@MainActor
private func measureActions() -> CGFloat {
    measureWidth(JobRowActions(isSkipped: false, onDecide: { _ in }, onRestore: {}))
}

@MainActor
private func measureTitle(_ title: String) -> CGFloat {
    measureWidth(Text(title).fontWeight(.semibold))
}

@MainActor
@Test func theJobColumnGetsTheRoomTheInspectorLeaves() {
    #expect(InspectorWidth.ideal == 420)
    #expect(InspectorWidth.min == 360)
    #expect(InspectorWidth.max == 560)
    // About 324 pt of text in the mockup.
    let textRoom = jobColumn - measureWidth(Monogram("Copperleaf Health")) - Space.m
    #expect(textRoom >= 324, "The Job column leaves \(textRoom) pt for the title and its context")
}

/// AppKit's header margins, as measured on QA's screenshot at AppKit's own
/// 17 pt spacing: 10 pt before a header's title, and after it 11 pt, or 26 pt
/// with the sort indicator. They may shrink with the spacing; the columns'
/// widths don't count on it.
@MainActor
@Test func theWordAndNumberColumnsLeaveTheirHeadersRoom() {
    // The headers' 11 pt system font.
    let font = NSFont.systemFont(ofSize: NSFont.smallSystemFontSize)
    func measureHeader(_ title: String) -> CGFloat {
        (title as NSString).size(withAttributes: [.font: font]).width
    }
    let columns: [(title: String, width: CGFloat, after: CGFloat)] = [
        ("Match", JobRowLayout.matchWidth, 11),
        ("Screen", JobRowLayout.screenWidth, 11),
        ("Take-home", JobRowLayout.takeHomeWidth, 11),
        // The table opens sorted by Posted.
        ("Posted", JobRowLayout.postedWidth, 26),
    ]
    for column in columns {
        let needed = 10 + measureHeader(column.title) + column.after
        #expect(needed > 40, "Nothing was measured for \(column.title)")
        #expect(column.width + JobRowLayout.cellSpacing >= needed, "\(column.title)'s header needs \(needed) pt")
    }
}

/// The seeded hub's widest rows: titles with an unseen dot and a chip, and
/// the longest company and location; Copperleaf's last, the row QA selects.
private func makeWidestRows() -> [JobListItem] {
    [
        makeItem("Senior Software Engineer, iOS", company: "Orchard Mesa Analytics", location: "Remote (Americas)", phase: "Interviewing", unseenUpdates: 1),
        makeItem("Senior Swift Engineer, Operator App", company: "Tidewell Robotics", location: "Remote, worldwide", phase: "Saved"),
        makeItem("Senior iOS Engineer, Carrier App", company: "Larkspur Freight", location: "Remote (LATAM)", phase: "Applied", unseenUpdates: 1),
        makeItem("Senior Platform Engineer, Swift", company: "Quillfeather Systems", location: "Remote, worldwide", phase: "Applied", unseenUpdates: 1),
        makeItem("Senior iOS Engineer, Patient App", company: "Copperleaf Health", location: "Remote (Americas)"),
    ]
}

@MainActor
@Test func titlesAndTheirContextShowInFullOnRowsAtRest() {
    for item in makeWidestRows() {
        let width = measureWidth(makeCell(item, isSelected: false))
        // The title alone is this wide; less means nothing was measured.
        #expect(width > measureTitle(item.job.title))
        #expect(width <= jobColumn, "\(item.job.title) needs \(width) pt; the Job column is \(jobColumn) pt")
    }
}

/// The room a selected row leaves its title: the Job column less what the
/// title line has beside it, measured on a cell without a context line, which
/// would otherwise set the cell's width.
@MainActor
private func getTitleRoomWhenSelected(_ title: String, phase: String? = nil) -> CGFloat {
    let cell = makeCell(makeItem(title, company: nil, location: nil, phase: phase), isSelected: true)
    let titleWidth = measureTitle(title)
    let width = measureWidth(cell)
    #expect(width > titleWidth)
    return jobColumn - (width - titleWidth)
}

@MainActor
@Test func theSelectedRowShowsItsTitleAndContextInFullBesideItsActions() {
    // Three small keys, as the design draws them.
    let actions = measureActions()
    #expect(actions > 3 * RowActionButtonStyle.size.width)
    #expect(actions <= 3 * RowActionButtonStyle.size.width + 2 * Space.xs + 1, "The row's actions take \(actions) pt")

    let items = [
        makeItem("Senior iOS Engineer, Patient App", company: "Copperleaf Health", location: "Remote (Americas)"),
        makeItem("Engineering Manager, iOS", company: "Brightwater Labs", location: "Remote (Americas)"),
    ]
    for item in items {
        let width = measureWidth(makeCell(item, isSelected: true))
        #expect(width > measureTitle(item.job.title) + actions)
        #expect(width <= jobColumn, "Selected, \(item.job.title) needs \(width) pt; the Job column is \(jobColumn) pt")
    }

    // Beside the actions and the whole "In Saved" chip, at least "Senior iOS
    // Engineer" shows, not "…".
    let patientAppRoom = getTitleRoomWhenSelected("Senior iOS Engineer, Patient App", phase: "Saved")
    #expect(patientAppRoom >= measureTitle("Senior iOS Engineer"), "The title gets \(patientAppRoom) pt beside its actions and chip")
}

/// A hovered or selected row's chips keep their words beside its actions:
/// the cell grows by the actions alone. Before TP-1135 the "In Applied" chip
/// shrank to its symbol, and color stood alone.
@MainActor
@Test func aHoveredOrSelectedRowsChipsKeepTheirWords() {
    let actions = measureActions()
    let carrierApp = makeItem("Senior iOS Engineer, Carrier App", company: nil, location: nil, phase: "Applied", unseenUpdates: 1)
    var skipped = makeItem("Senior iOS Engineer, Carrier App", company: nil, location: nil)
    skipped.job.dismissalReason = "Too far from home"
    for item in [carrierApp, skipped] {
        let atRest = measureWidth(makeCell(item, isSelected: false))
        for (isSelected, isHovered) in [(false, true), (true, false)] {
            let width = measureWidth(makeCell(item, isSelected: isSelected, isHovered: isHovered))
            #expect(
                abs(width - (atRest + Space.s + actions)) < 1,
                "Hovered \(isHovered), selected \(isSelected): \(width) pt, at rest \(atRest) pt, actions \(actions) pt"
            )
        }
    }
    // Beside the inspector, the dot and the whole "In Applied" chip still
    // leave the title at least "Senior iOS Engineer".
    let cell = makeCell(carrierApp, isSelected: false, isHovered: true)
    let titleRoom = jobColumn - (measureWidth(cell) - measureTitle(carrierApp.job.title))
    #expect(titleRoom >= measureTitle("Senior iOS Engineer"), "The title gets \(titleRoom) pt beside its actions and chip")
}

/// With the inspector closed, the hovered row has the room for its whole
/// title, its dot, its chip and its actions: nothing is cut.
@MainActor
@Test func aHoveredRowShowsInFullAtFullWidth() {
    for item in makeWidestRows() {
        let width = measureWidth(makeCell(item, isSelected: false, isHovered: true))
        #expect(width > measureTitle(item.job.title) + measureActions())
        #expect(width <= fullWidthJobColumn, "Hovered, \(item.job.title) needs \(width) pt; the Job column is \(fullWidthJobColumn) pt")
    }
}

/// The widest rows drawn offscreen at the Job column's width, at rest,
/// Carrier App's hovered and Copperleaf's selected, in both appearances.
/// With HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the PNGs are
/// written there.
@MainActor
@Test func theWidestRowsDrawAtTheJobColumnsWidth() throws {
    let rows = makeWidestRows()
    for colorScheme in [ColorScheme.light, .dark] {
        let content = VStack(spacing: Space.s) {
            ForEach(rows) { item in
                let isSelected = item.id == rows.last?.id
                makeCell(item, isSelected: isSelected, isHovered: item.job.title.hasSuffix("Carrier App"))
                    .background(isSelected ? Color.hubAccent.opacity(0.13) : .clear)
            }
        }
        .frame(width: jobColumn)
        .padding(Space.s)
        .background(Color(nsColor: .windowBackgroundColor))
        .tint(.hubAccent)
        .environment(\.colorScheme, colorScheme)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        let image = try #require(renderer.cgImage)
        #expect(image.width == Int((jobColumn + 2 * Space.s) * 2))
        try writeSnapshot(image, named: colorScheme == .light ? "light" : "dark")
    }
}

private func writeSnapshot(_ image: CGImage, named name: String) throws {
    guard let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
          let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])
    else { return }
    let url = URL(filePath: folder)
    try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    try png.write(to: url.appending(path: "jobs-widest-rows-\(name).png"))
}

/// A table like the Jobs table in a scroll view, unwindowed: a resizable Job
/// column, the four fixed ones, and a hidden optional one at the end. At
/// 732 pt its columns start short of the right edge, as AppKit leaves them.
@MainActor
private func makeTable(width: CGFloat) -> (scrollView: NSScrollView, table: NSTableView) {
    let scrollView = NSScrollView(frame: NSRect(x: 0, y: 0, width: width, height: 400))
    let table = NSTableView(frame: scrollView.contentView.bounds)
    let job = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("job"))
    job.minWidth = JobRowLayout.jobMinWidth
    job.maxWidth = 10000
    job.width = 300
    table.addTableColumn(job)
    let fixedWidths = [JobRowLayout.matchWidth, JobRowLayout.screenWidth, JobRowLayout.takeHomeWidth, JobRowLayout.postedWidth]
    for (index, fixedWidth) in fixedWidths.enumerated() {
        let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("fixed\(index)"))
        column.minWidth = fixedWidth
        column.maxWidth = fixedWidth
        column.width = fixedWidth
        table.addTableColumn(column)
    }
    let optional = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("optional"))
    optional.width = 130
    optional.isHidden = true
    table.addTableColumn(optional)
    scrollView.documentView = table
    scrollView.tile()
    table.intercellSpacing = NSSize(width: 17, height: 0)
    return (scrollView, table)
}

@MainActor
@Test func theCellSpacingItFreesGoesToTheJobColumn() {
    let (scrollView, table) = makeTable(width: 732)
    let clipWidth = scrollView.contentView.bounds.width
    #expect(clipWidth > 0)

    table.setCellSpacing(JobRowLayout.cellSpacing)

    #expect(table.intercellSpacing.width == JobRowLayout.cellSpacing)
    #expect(table.tableColumns[0].width > 300)
    // The shown columns end as far from the right edge as they start from the left.
    let start = table.rect(ofColumn: 0).minX
    let end = table.rect(ofColumn: 4).maxX
    #expect(abs(clipWidth - end - start) < 0.5, "The columns run \(start) to \(end) in \(clipWidth) pt")
    #expect(table.tableColumns[1...4].map(\.width) == [
        JobRowLayout.matchWidth, JobRowLayout.screenWidth, JobRowLayout.takeHomeWidth, JobRowLayout.postedWidth,
    ])
    #expect(table.tableColumns[5].width == 130)

    // Set again, it changes nothing.
    let jobWidth = table.tableColumns[0].width
    table.setCellSpacing(JobRowLayout.cellSpacing)
    #expect(table.tableColumns[0].width == jobWidth)
}

@MainActor
@Test func aNarrowTableKeepsTheJobColumnAtItsNarrowest() {
    let (_, table) = makeTable(width: 400)
    table.setCellSpacing(JobRowLayout.cellSpacing)
    #expect(table.intercellSpacing.width == JobRowLayout.cellSpacing)
    #expect(table.tableColumns[0].width == JobRowLayout.jobMinWidth)
}

@MainActor
@Test func aTableWithoutWidthWaitsForOne() {
    let (_, table) = makeTable(width: 0)
    table.setCellSpacing(JobRowLayout.cellSpacing)
    #expect(table.intercellSpacing.width == 17)
}
