// The pattern board: each UI pattern the proposal uses, with what it's for,
// a specimen, and what it replaces.
import SwiftUI

/// Draws a view at a fraction of its size, clipped to the result.
struct Scaled<Content: View>: View {
    let scale: CGFloat
    let width: CGFloat
    let height: CGFloat
    @ViewBuilder let content: Content
    var body: some View {
        content
            .frame(width: width, height: height, alignment: .topLeading)
            .scaleEffect(scale, anchor: .topLeading)
            .frame(width: width * scale, height: height * scale, alignment: .topLeading)
            .clipped()
    }
}

struct PatternTile<Specimen: View>: View {
    let number: String
    let name: String
    let use: String
    let replaces: String
    @ViewBuilder let specimen: Specimen
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text(number).font(.ui(11, .bold)).monospacedDigit().foregroundStyle(Tone.accent.color)
                Text(name).font(.ui(15, .semibold)).foregroundStyle(ink)
            }
            Text(use).font(.ui(12.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            specimen
                .padding(Space.l)
                .frame(maxWidth: .infinity, minHeight: 150, alignment: .center)
                .background(board, in: RoundedRectangle(cornerRadius: Radius.card))
            HStack(alignment: .firstTextBaseline, spacing: 5) {
                Text("Replaces").font(.ui(11, .semibold)).foregroundStyle(tertiaryInk)
                Text(replaces).font(.ui(11.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(Space.l + 2)
        .frame(width: 520, alignment: .topLeading)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
    }
}

/// Three rectangles: a list, the list with the inspector, the page.
struct DepthsDiagram: View {
    var body: some View {
        HStack(spacing: Space.s) {
            frame(label: "List", inspector: false, page: false)
            arrow("Select")
            frame(label: "Inspector", inspector: true, page: false)
            arrow("Return")
            frame(label: "Page", inspector: false, page: true)
        }
    }

    private func frame(label: String, inspector: Bool, page: Bool) -> some View {
        VStack(spacing: 5) {
            HStack(spacing: 2) {
                RoundedRectangle(cornerRadius: 2).fill(sidebarBackground).frame(width: 16)
                if page {
                    VStack(alignment: .leading, spacing: 3) {
                        RoundedRectangle(cornerRadius: 1).fill(ink.opacity(0.6)).frame(width: 40, height: 4)
                        HStack(alignment: .top, spacing: 3) {
                            VStack(alignment: .leading, spacing: 2) {
                                ForEach(0..<6, id: \.self) { _ in RoundedRectangle(cornerRadius: 1).fill(ink.opacity(0.18)).frame(height: 2.5) }
                            }
                            RoundedRectangle(cornerRadius: 2).fill(Tone.accent.fill).frame(width: 18, height: 30)
                        }
                    }
                    .padding(4)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                    .background(surface)
                } else {
                    VStack(spacing: 3) {
                        ForEach(0..<6, id: \.self) { row in
                            RoundedRectangle(cornerRadius: 1).fill(row == 1 && inspector ? Tone.accent.color.opacity(0.5) : ink.opacity(0.15)).frame(height: 4)
                        }
                    }
                    .padding(4)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .background(surface)
                    if inspector {
                        RoundedRectangle(cornerRadius: 2).fill(Tone.accent.fill).frame(width: 26)
                    }
                }
            }
            .frame(width: 104, height: 66)
            .padding(2)
            .background(separator, in: RoundedRectangle(cornerRadius: 5))
            Text(label).font(.ui(11, .medium)).foregroundStyle(secondaryInk)
        }
    }

    private func arrow(_ key: String) -> some View {
        VStack(spacing: 2) {
            Image(systemName: "arrow.right").font(.system(size: 11, weight: .semibold)).foregroundStyle(tertiaryInk)
            Text(key).font(.ui(9.5)).foregroundStyle(tertiaryInk)
        }
    }
}

struct PatternBoard: View {
    var body: some View {
        Board(
            eyebrow: "Level 1 · UI patterns",
            title: "Twelve patterns every page is built from",
            subtitle: "The first redesign gave the app tokens and components: tones, chips, sections, an action bar. These patterns sit one level up: how a page, a list, a detail and a form are put together, so a new page is assembled from them instead of designed again.",
            width: 1740
        ) {
            Grid(alignment: .topLeading, horizontalSpacing: Space.xl, verticalSpacing: Space.xl) {
                GridRow {
                    PatternTile(number: "P1", name: "Page header", use: "Every page's controls, in one place: scopes with counts on the left, search and one Add on the right, the filters that are on as chips under them.", replaces: "Window toolbar items, the PageBar, filter popovers, status menus") {
                        Scaled(scale: 0.8, width: 560, height: 100) {
                            PageHeader(scopes: [("Open", "48"), ("Later", "6"), ("Skipped", "112")], selectedScope: "Open", filters: ["Passes screen", "Remote"], search: "Search jobs")
                                .padding(.top, Space.m).background(surface)
                        }
                    }
                    PatternTile(number: "P2", name: "List, inspector, page", use: "Three depths for one thing. Select a row to triage it in the inspector; Return, double-click or ⤢ opens it as a page to read and work on; Esc goes back to the row.", replaces: "The inspector as the only detail view, and the Companies page's fixed panel") {
                        DepthsDiagram()
                    }
                    PatternTile(number: "P3", name: "Reader", use: "Long text: postings, briefs, interview packs, the profile. Real headings and lists, 13–14 pt at 1.3 line height, a 600–640 pt measure, an outline to jump, ⌘F, and marks tied to verdicts.", replaces: "Text(description) at the inspector's width") {
                        PostingReader(size: 12, limit: 7).frame(width: 440)
                    }
                }
                GridRow {
                    PatternTile(number: "P4", name: "Verdict strip", use: "The few judgments a decision rests on, side by side under the title, each a label, a symbol and a word in its tone. A cell opens its evidence.", replaces: "Two or three chips in the header") {
                        VerdictStrip().frame(width: 450)
                    }
                    PatternTile(number: "P5", name: "Card", use: "A group in a detail or on a page: a title, a quiet meta, one trailing link, then the content. Three cards at most before a tab or a page takes over.", replaces: "HubSection headings over long stacks") {
                        Card(title: "People", meta: "2 you know at Northwind", trailing: "All 6") {
                            Text("Alex Kim · Engineering Manager").font(.ui(12.5)).foregroundStyle(ink)
                        }
                        .frame(width: 420)
                    }
                    PatternTile(number: "P6", name: "Exceptions first", use: "A list of checks shows what doesn't pass, with its quote and a link to its source, and folds what passes into one row of ✓ chips.", replaces: "Every check as a row of equal weight") {
                        ScreenCard().frame(width: 440)
                    }
                }
                GridRow {
                    PatternTile(number: "P7", name: "Two-line row", use: "Tables of things you decide on: a monogram, the name over its context, then three or four columns of words in tones. Hover and selection show the row's own actions.", replaces: "One-line rows of equal columns with a chip first") {
                        Scaled(scale: 0.66, width: 690, height: 92) {
                            VStack(spacing: 0) {
                                ProposedJobRow(row: SampleJobs.rows[0], isSelected: true)
                                ProposedJobRow(row: SampleJobs.rows[1])
                            }
                            .padding(2)
                        }
                    }
                    PatternTile(number: "P8", name: "One status line", use: "A card or row says the most urgent thing about it, in one line, in its tone; everything else waits for the inspector.", replaces: "Pipeline cards with up to seven lines in five colors") {
                        HStack(alignment: .top, spacing: Space.m) {
                            ProposedCard(card: SampleBoard.phases[0].1[0]).frame(width: 214)
                            ProposedCard(card: SampleBoard.phases[2].1[0]).frame(width: 214)
                        }
                    }
                    PatternTile(number: "P9", name: "Tab strip", use: "Tabs in an inspector, a page and Settings: text with an accent underline, a count or a dot where a tab holds news. Left-aligned, full width, never resized by its content.", replaces: "Segmented pickers at their natural width") {
                        TabStrip(tabs: [("Overview", nil), ("Posting", nil), ("Prep", "2"), ("Session", "•")], selected: "Overview").frame(width: 420)
                    }
                }
                GridRow {
                    PatternTile(number: "P10", name: "Token field", use: "Any list a person edits: each value a token to remove, a field for the next. Tokens that rule something out are red, ones that let it in green.", replaces: "Comma-separated text fields") {
                        VStack(alignment: .leading, spacing: Space.s) {
                            TokenField(tokens: ["Brazil", "LATAM", "Americas", "worldwide"], tone: .positive)
                            TokenField(tokens: ["US only", "EU only"], tone: .negative)
                        }
                        .frame(width: 420)
                    }
                    PatternTile(number: "P11", name: "Save bar", use: "A form you edit and then save rises a bar from the bottom once something changed: how many changes, Revert, Save (⌘S). It stays until saved.", replaces: "Revert and Save at the end of a long form") {
                        HStack(spacing: Space.m) {
                            Circle().fill(Tone.accent.color).frame(width: 7, height: 7)
                            Text("3 unsaved changes").font(.ui(12.5, .medium)).foregroundStyle(ink)
                            Spacer()
                            SecondaryButton(title: "Revert")
                            PrimaryButton(title: "Save", shortcut: "⌘S")
                        }
                        .padding(.horizontal, Space.l).padding(.vertical, 10)
                        .frame(width: 420)
                        .background(surface, in: Capsule())
                        .overlay(Capsule().strokeBorder(separator))
                        .shadow(color: .black.opacity(0.1), radius: 10, y: 4)
                    }
                    PatternTile(number: "P12", name: "Status row", use: "An account, a service or the hub itself: a tile, the name, its state as a dot and a word, one action. Help sits behind ⓘ. The sidebar's foot uses the same row for the hub.", replaces: "Form sections with a paragraph of footer each") {
                        SettingsRowsSample().frame(width: 430)
                    }
                }
            }
        }
    }
}
