// The window: the Jobs page with a job open, as built today and as
// proposed, with the page header, the table rows and the sidebar's foot.
import SwiftUI

struct JobRowData: Identifiable {
    let id: Int
    let title: String
    let company: String
    let monogram: String
    let hue: UInt32
    let location: String
    let match: (String, Tone)?
    let screen: (String, Tone, String)
    let takeHome: String
    let posted: String
    var isNew = false
    var phase: String?
    var unseen = false
}

enum SampleJobs {
    static let rows: [JobRowData] = [
        JobRowData(id: 0, title: "Senior Product Engineer", company: "Northwind", monogram: "N", hue: 0x0E7C86, location: "Remote, Americas",
                   match: ("Strong", .positive), screen: ("2 unclear", .caution, "questionmark.circle.fill"), takeHome: "R$ 31k", posted: "2 d", isNew: true, unseen: true),
        JobRowData(id: 1, title: "Staff Frontend Engineer", company: "Globex", monogram: "G", hue: 0x7A4FD1, location: "Remote, LATAM",
                   match: ("Possible", .accent), screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "R$ 36k", posted: "2 d", isNew: true),
        JobRowData(id: 2, title: "Senior Engineer, Growth", company: "Lumen Health", monogram: "L", hue: 0xC2410C, location: "Remote, Worldwide",
                   match: nil, screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "—", posted: "3 d", isNew: true),
        JobRowData(id: 3, title: "Full-Stack Engineer, Payments", company: "Initech", monogram: "I", hue: 0x2563EB, location: "Remote, Brazil",
                   match: ("Strong", .positive), screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "R$ 28k", posted: "4 d", phase: "Applied"),
        JobRowData(id: 4, title: "Senior Software Engineer, Platform", company: "Umbrella Labs", monogram: "U", hue: 0xB91C1C, location: "Remote, Worldwide",
                   match: ("Stretch", .caution), screen: ("1 unclear", .caution, "questionmark.circle.fill"), takeHome: "R$ 33k", posted: "5 d"),
        JobRowData(id: 5, title: "Frontend Engineer II", company: "Brightline", monogram: "B", hue: 0x0F766E, location: "Remote, Americas",
                   match: ("Possible", .accent), screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "R$ 24k", posted: "6 d"),
        JobRowData(id: 6, title: "Product Engineer", company: "Acme Robotics", monogram: "A", hue: 0x9333EA, location: "São Paulo, hybrid",
                   match: ("Mismatch", .neutral), screen: ("Where", .negative, "xmark.circle.fill"), takeHome: "R$ 22k", posted: "1 w"),
        JobRowData(id: 7, title: "Senior React Engineer", company: "Cobalt", monogram: "C", hue: 0x1D4ED8, location: "Remote, US only",
                   match: nil, screen: ("Where", .negative, "xmark.circle.fill"), takeHome: "—", posted: "1 w"),
        JobRowData(id: 8, title: "Senior Frontend Engineer", company: "Vandelay", monogram: "V", hue: 0x4D7C0F, location: "Remote, Americas",
                   match: ("Possible", .accent), screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "R$ 27k", posted: "1 w"),
        JobRowData(id: 9, title: "Software Engineer, Checkout", company: "Globex", monogram: "G", hue: 0x7A4FD1, location: "Remote, LATAM",
                   match: ("Stretch", .caution), screen: ("Pay", .negative, "xmark.circle.fill"), takeHome: "R$ 17k", posted: "2 w"),
    ]
}

// MARK: - Today

/// The Jobs page as built: an icon bar over the table, the screen as the
/// first column, and the window's inspector.
struct CurrentJobsPage: View {
    var body: some View {
        VStack(spacing: 0) {
            TitleBar(title: "Jobs", subtitle: "48 jobs · 31 pass the screen")
            // The PageBar: icon-only controls, right-aligned.
            HStack(spacing: Space.s) {
                Spacer()
                IconButton(symbol: "line.3.horizontal.decrease.circle")
                HStack(spacing: 4) {
                    Image(systemName: "tray.full").font(.system(size: 11))
                    Text("Open").font(.ui(12))
                    Image(systemName: "chevron.down").font(.system(size: 8, weight: .bold))
                }
                .padding(.horizontal, 9).padding(.vertical, 4).background(fill, in: Capsule()).foregroundStyle(ink)
                IconButton(symbol: "tablecells")
                IconButton(symbol: "plus")
                SearchBox(prompt: "Title, location or company", width: 170)
            }
            .padding(.horizontal, Space.m).padding(.vertical, Space.s)
            .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
            .overlay(alignment: .leading) { Marker(number: 2).offset(x: 12) }
            VStack(spacing: 0) {
                header
                ForEach(SampleJobs.rows) { row in
                    currentRow(row)
                }
            }
            .overlay(alignment: .topLeading) { Marker(number: 3).offset(x: -10, y: 30) }
            Spacer(minLength: 0)
        }
        .background(surface)
    }

    private var header: some View {
        HStack(spacing: 0) {
            cell("Screen", 78)
            cell("Title", 250)
            cell("Company", 120)
            cell("Location", 150)
            cell("First seen", 90)
            Spacer(minLength: 0)
        }
        .font(.ui(11, .medium)).foregroundStyle(secondaryInk)
        .frame(height: 26)
        .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
    }

    private func cell(_ text: String, _ width: CGFloat) -> some View {
        Text(text).frame(width: width, alignment: .leading).padding(.leading, 10)
            .overlay(alignment: .leading) { Rectangle().fill(separator).frame(width: 1, height: 14) }
    }

    private func currentRow(_ row: JobRowData) -> some View {
        let isSelected = row.id == 0
        let screenWord = row.screen.1 == .positive ? "Passes" : row.screen.1 == .caution ? "Unclear" : "Fails"
        return HStack(spacing: 0) {
            Chip(text: screenWord, tone: row.screen.1).frame(width: 78, alignment: .leading).padding(.leading, 10)
            HStack(spacing: 6) {
                if row.unseen { UnseenDot() }
                Text(row.title).lineLimit(1)
                if row.isNew { Chip(text: "New", tone: .accent) }
            }
            .frame(width: 250, alignment: .leading).padding(.leading, 10)
            Text(row.company).frame(width: 120, alignment: .leading).padding(.leading, 10)
            Text(row.location).lineLimit(1).frame(width: 150, alignment: .leading).padding(.leading, 10)
            Text(row.posted == "2 d" ? "4 Oct 2026" : "1 Oct 2026").frame(width: 90, alignment: .leading).padding(.leading, 10)
            Spacer(minLength: 0)
        }
        .font(.ui(12.5)).foregroundStyle(isSelected ? .white : ink)
        .frame(height: 26)
        .background(isSelected ? Tone.accent.color : (row.id % 2 == 1 ? Color.black.opacity(0.025) : .clear))
    }
}

// MARK: - Proposed

/// A table row as proposed: a status glyph, the job and its company on two
/// lines, the match and the screen as words, then take-home and age; the
/// row's own actions show on hover and selection.
struct ProposedJobRow: View {
    let row: JobRowData
    var isSelected = false
    var isHovered = false
    var body: some View {
        HStack(spacing: 0) {
            Monogram(letters: row.monogram, hue: Color(hex: row.hue), size: 26).padding(.trailing, Space.m)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 6) {
                    Text(row.title).font(.ui(13, .semibold)).foregroundStyle(ink).lineLimit(1)
                    if row.unseen { UnseenDot() }
                    if let phase = row.phase { Chip(text: "In \(phase)", tone: .accent, symbol: "rectangle.split.3x1") }
                }
                Text("\(row.company) · \(row.location)").font(.ui(11.5)).foregroundStyle(secondaryInk).lineLimit(1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if isHovered || isSelected {
                HStack(spacing: 4) {
                    rowAction("arrow.up.forward", "P")
                    rowAction("clock", "L")
                    rowAction("eye.slash", "S")
                }
                .padding(.trailing, Space.m)
            }
            Group {
                if let match = row.match {
                    Text(match.0).font(.ui(12, .medium)).foregroundStyle(match.1.color)
                } else {
                    Text("Not briefed").font(.ui(12)).foregroundStyle(tertiaryInk)
                }
            }
            .frame(width: 84, alignment: .leading)
            HStack(spacing: 4) {
                Image(systemName: row.screen.2).font(.system(size: 11)).foregroundStyle(row.screen.1.color)
                Text(row.screen.0).font(.ui(12)).foregroundStyle(row.screen.1 == .positive ? secondaryInk : row.screen.1.color)
            }
            .frame(width: 94, alignment: .leading)
            Text(row.takeHome).font(.ui(12)).monospacedDigit().foregroundStyle(row.takeHome == "—" ? tertiaryInk : ink).frame(width: 66, alignment: .trailing)
            Text(row.posted).font(.ui(12)).monospacedDigit().foregroundStyle(secondaryInk).frame(width: 56, alignment: .trailing)
        }
        .padding(.horizontal, Space.m)
        .frame(height: 44)
        .background(isSelected ? Tone.accent.color.opacity(0.10) : isHovered ? fillSubtle : .clear, in: RoundedRectangle(cornerRadius: 7))
        .overlay(alignment: .leading) {
            if isSelected { Capsule().fill(Tone.accent.color).frame(width: 3, height: 26).offset(x: 1) }
        }
    }

    private func rowAction(_ symbol: String, _ key: String) -> some View {
        Image(systemName: symbol).font(.system(size: 11, weight: .medium)).foregroundStyle(ink)
            .frame(width: 26, height: 22).background(surface, in: RoundedRectangle(cornerRadius: 6))
            .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(separator))
    }
}

struct ProposedJobsTable: View {
    var rowCount = 10
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                Text("Job").frame(maxWidth: .infinity, alignment: .leading).padding(.leading, 38)
                Text("Match").frame(width: 84, alignment: .leading)
                Text("Screen").frame(width: 94, alignment: .leading)
                Text("Take-home").frame(width: 66, alignment: .trailing)
                HStack(spacing: 2) {
                    Text("Posted")
                    Image(systemName: "chevron.down").font(.system(size: 7, weight: .bold))
                }
                .frame(width: 56, alignment: .trailing)
            }
            .font(.ui(11, .medium)).foregroundStyle(secondaryInk)
            .padding(.horizontal, Space.m)
            .frame(height: 28)
            groupHeader("New since yesterday", "3")
            ForEach(SampleJobs.rows.prefix(3)) { row in
                ProposedJobRow(row: row, isSelected: row.id == 0)
            }
            groupHeader("Earlier this week", "45")
            ForEach(SampleJobs.rows.dropFirst(3).prefix(max(0, rowCount - 3))) { row in
                ProposedJobRow(row: row, isHovered: row.id == 4)
            }
        }
        .padding(.horizontal, Space.s)
    }

    private func groupHeader(_ title: String, _ count: String) -> some View {
        HStack(spacing: 6) {
            Text(title).font(.ui(11.5, .semibold)).foregroundStyle(ink)
            Text(count).font(.ui(11.5)).foregroundStyle(tertiaryInk)
            Spacer()
        }
        .padding(.horizontal, Space.m)
        .padding(.top, Space.m).padding(.bottom, 4)
    }
}

struct ProposedJobsPage: View {
    var body: some View {
        VStack(spacing: 0) {
            TitleBar(title: "Jobs", subtitle: "48 open · 31 pass the screen · 3 new")
            PageHeader(
                scopes: [("Open", "48"), ("Later", "6"), ("Skipped", "112"), ("All", nil)], selectedScope: "Open",
                filters: ["Passes screen", "Remote"], search: "Search jobs", addTitle: "Add", trailingNote: "Grouped by first seen"
            )
            .overlay(alignment: .topLeading) { Marker(number: 2).offset(x: -4, y: 4) }
            .overlay(alignment: .bottomLeading) { Marker(number: 3).offset(x: -4, y: -8) }
            ProposedJobsTable()
                .overlay(alignment: .topLeading) { Marker(number: 4).offset(x: -4, y: 70) }
            Spacer(minLength: 0)
        }
        .background(surface)
    }
}

// MARK: - Windows

struct CurrentWindow: View {
    var body: some View {
        WindowFrame(width: 1360, height: 760) {
            HStack(spacing: 0) {
                Sidebar(selected: "Jobs")
                    .overlay(alignment: .topTrailing) { Marker(number: 1).offset(x: -20, y: 368) }
                CurrentJobsPage().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                Rectangle().fill(separator).frame(width: 1)
                VStack(alignment: .leading, spacing: 0) {
                    InspectorBar(expands: false)
                    CurrentJobTop(tab: "Overview")
                    CurrentOverviewTab().withoutMarkers()
                }
                .frame(width: 420, height: 760, alignment: .top)
                .background(surface)
                .clipped()
                .overlay(alignment: .topLeading) { Marker(number: 5).offset(x: 8, y: 10) }
            }
        }
    }
}

struct ProposedWindow: View {
    var body: some View {
        WindowFrame(width: 1360, height: 760) {
            HStack(spacing: 0) {
                Sidebar(selected: "Jobs", proposed: true)
                    .overlay(alignment: .bottomTrailing) { Marker(number: 1).offset(x: -20, y: -110) }
                ProposedJobsPage().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                Rectangle().fill(separator).frame(width: 1)
                VStack(alignment: .leading, spacing: 0) {
                    InspectorBar()
                    ProposedJobTop(tab: "Overview").padding(.bottom, Space.xs)
                    ProposedOverviewTab().withoutMarkers()
                }
                .frame(width: 420, height: 760, alignment: .top)
                .background(board)
                .clipped()
                .overlay(alignment: .topTrailing) { Marker(number: 5).offset(x: -60, y: 10) }
            }
        }
    }
}

struct ShellBeforeAfter: View {
    var body: some View {
        Board(
            eyebrow: "App shell · top-level layout",
            title: "One frame for every page",
            subtitle: "Three columns stay: sidebar, page, inspector. What changes is that every page puts its controls in the same place, the hub's state has one home, and a row says enough to decide without opening it.",
            width: 1900
        ) {
            HStack(alignment: .top, spacing: 48) {
                VStack(alignment: .leading, spacing: Space.xxl) {
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Today", subtitle: "The Jobs page with a job open.", tone: .negative)
                        CurrentWindow()
                    }
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Proposed", subtitle: "The same page and the same job.", tone: .positive)
                        ProposedWindow()
                    }
                }
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("What changes").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Sessions take the sidebar's foot, and the hub's state is nowhere", fix: "A status footer: connected or not, what the local models are doing, a pause button, and the one session that waits for you. Sessions move to ⌘K and the jobs' Session tabs.")
                    ProblemNote(number: 2, problem: "Controls live in three places: the window toolbar, a right-aligned icon bar, or nowhere", fix: "Every page gets one header: scope tabs with counts on the left, search and one Add on the right.")
                    ProblemNote(number: 3, problem: "Filters hide in a popover; only the icon says one is on", fix: "Filters that are on show as chips you can remove; + Filter adds one.")
                    ProblemNote(number: 4, problem: "Rows are one line of equal columns, and the screen is the first thing you read", fix: "Two-line rows: the job and its company first, then Match, Screen, Take-home, Posted. Grouped by when the hub first saw them. Hover shows Pursue, Later, Skip.")
                    ProblemNote(number: 5, problem: "The inspector is the only way to see a job", fix: "It keeps triage, and its expand button (or Return) opens the job as a page.")
                    Divider().padding(.vertical, Space.s)
                    Text("Rules").font(.ui(15, .semibold)).foregroundStyle(ink)
                    rule("The window toolbar holds the page title and nothing else: items there reach over the inspector and have looped AppKit's layout before.")
                    rule("A page header never wraps: scopes, search and Add keep their width, and the chips row scrolls sideways.")
                    rule("The inspector is 420 pt by default, 360 to 560. Anything that needs more opens as a page.")
                    rule("Only Pipeline's badge is red, and only for overdue follow-ups. Other counts are grey.")
                }
                .frame(width: 400)
            }
        }
    }

    private func rule(_ text: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: "checkmark").font(.system(size: 10, weight: .bold)).foregroundStyle(Tone.accent.color)
            Text(text).font(.ui(12.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
    }
}
