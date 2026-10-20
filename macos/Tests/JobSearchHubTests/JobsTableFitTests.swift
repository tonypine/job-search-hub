import AppKit
@testable import JobSearchHub
@testable import JobSearchHubCore
import SwiftUI
import Testing

// The Jobs table beside the inspector, its job cells measured offscreen in a
// hosting view that no window holds. In a 1400 pt window with the inspector
// at 480 pt, TP-659's epic review found every title cut ("Senior iOS Eng…"),
// the Posted header cut to "Pos…", and the selected row's title drawn as "…"
// beside its actions and its "In Saved" chip (TP-1123). These check the Job
// column's room against what its rows need.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset, and the inspector at its ideal width.
private let contentColumn: CGFloat = 1400 - 248 - InspectorWidth.ideal

/// What the table takes besides its columns' widths, as measured on the
/// epic review's screenshot: 14 pt of row inset and 17 pt between the cells
/// of each of the five columns.
private let tableChrome: CGFloat = 14 + 5 * 17

/// The Job column's width, the room the fixed columns leave it.
private let jobColumn = contentColumn - tableChrome - JobRowLayout.fixedColumnsWidth

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

private func makeItem(_ title: String, company: String?, location: String?, phase: String? = nil) -> JobListItem {
    let seen = Date(timeIntervalSince1970: 0)
    let job = Job(id: UUID(), source: "manual", title: title, location: location, url: "https://example.com/job", firstSeenAt: seen, lastSeenAt: seen)
    var item = JobListItem(job: job, companyName: company, fit: JobFit(level: .good, checks: []), unseenUpdates: 0)
    item.pipelinePhase = phase
    return item
}

@MainActor
private func makeCell(_ item: JobListItem, isSelected: Bool) -> JobCell {
    JobCell(item: item, isSelected: isSelected, hover: JobRowHover(), isSkipped: false, onDecide: { _ in }, onRestore: {})
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
    // About 324 pt of text in the mockup; less than 280 cuts its titles.
    let textRoom = jobColumn - measureWidth(Monogram("Copperleaf Health")) - Space.m
    #expect(textRoom >= 280, "The Job column leaves \(textRoom) pt for the title and its context")
}

@MainActor
@Test func titlesAndTheirContextShowInFullOnRowsAtRest() {
    let items = [
        makeItem("Senior iOS Engineer", company: "Brightwater Labs", location: "Remote (Americas)", phase: "Applied"),
        makeItem("Senior Platform Engineer", company: "Quillfeather Systems", location: "Remote, Americas", phase: "Applied"),
        makeItem("Senior iOS Engineer, Patient App", company: "Copperleaf Health", location: "Remote (Americas)"),
    ]
    for item in items {
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
@Test func theSelectedRowKeepsItsTitleReadableBesideItsActions() {
    let managerRoom = getTitleRoomWhenSelected("Engineering Manager, iOS")
    #expect(managerRoom >= measureTitle("Engineering Manager, iOS"), "The title gets \(managerRoom) pt beside its actions")

    // The chip shrinks to its symbol beside the actions...
    let patientApp = makeItem("Senior iOS Engineer, Patient App", company: "Copperleaf Health", location: "Remote (Americas)", phase: "Saved")
    let atRest = measureWidth(makeCell(patientApp, isSelected: false))
    let selected = measureWidth(makeCell(patientApp, isSelected: true))
    let actions = measureWidth(JobRowActions(isSkipped: false, onDecide: { _ in }, onRestore: {}))
    #expect(selected < atRest + Space.m + actions - 20, "The \"In Saved\" chip keeps its text beside the actions")
    // ...so at least "Senior iOS Engineer" shows, not "…".
    let patientAppRoom = getTitleRoomWhenSelected(patientApp.job.title, phase: "Saved")
    #expect(patientAppRoom >= measureTitle("Senior iOS Engineer"), "The title gets \(patientAppRoom) pt beside its actions and chip")
}
