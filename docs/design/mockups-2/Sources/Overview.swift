// The job's Overview tab, before and after, and the job opened as a page.
import SwiftUI

// MARK: - Overview today

struct CurrentOverviewTab: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.l) {
            VStack(alignment: .leading, spacing: Space.s) {
                HStack {
                    Text("Brief").font(.ui(13, .semibold)).foregroundStyle(ink)
                    Spacer()
                    Text("by the local model").font(.ui(11)).foregroundStyle(secondaryInk)
                }
                Label("Your knowledge base changed since", systemImage: "clock.arrow.circlepath").font(.ui(11)).foregroundStyle(Tone.caution.color)
                Text("Your payments and checkout cases map straight onto their rebuild, and the contractor pay in USD likely clears your take-home target. The gaps are GraphQL federation and the years they ask for.")
                    .font(.ui(13)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                    .marker(1, x: -30)
                Text("Strengths").font(.ui(12, .semibold)).foregroundStyle(secondaryInk)
                point("plus.circle.fill", .positive, "Led a checkout rewrite that lifted conversion", "Checkout rebuild")
                point("plus.circle.fill", .positive, "Five years of React and TypeScript in product teams", "React; TypeScript")
                point("plus.circle.fill", .positive, "Works remote with US teams today", "Remote work")
                Text("Weaknesses").font(.ui(12, .semibold)).foregroundStyle(secondaryInk)
                point("minus.circle.fill", .caution, "No production GraphQL federation", "GraphQL")
                point("minus.circle.fill", .caution, "Five years in React against the six they ask", "React")
                SecondaryButton(title: "Write full brief", symbol: "sparkles").padding(.top, Space.xs)
            }
            .marker(2, x: -30, y: 122)
            VStack(alignment: .leading, spacing: Space.s) {
                HStack {
                    Text("Screen").font(.ui(13, .semibold)).foregroundStyle(ink)
                    Spacer()
                    Chip(text: "Unclear", tone: .caution)
                }
                VerdictRow(verdict: .pass, name: "Role", reason: "Product engineer")
                VerdictRow(verdict: .pass, name: "Where they hire", reason: "Americas")
                Evidence(text: "fully remote across the Americas")
                VerdictRow(verdict: .pass, name: "Stack", reason: "React, TypeScript, Node.js")
                VerdictRow(verdict: .unclear, name: "Years", reason: "Asks 6+, you have 5")
                Evidence(text: "6+ years building web products with React")
                VerdictRow(verdict: .unclear, name: "Timezone", reason: "4 h overlap with US Eastern")
                Evidence(text: "At least 4 hours of overlap with US Eastern time.")
                VerdictRow(verdict: .pass, name: "Pay", reason: "About R$ 31k a month take-home")
            }
            .marker(3, x: -30)
            VStack(alignment: .leading, spacing: Space.s) {
                HStack {
                    Text("People").font(.ui(13, .semibold)).foregroundStyle(ink)
                    Spacer()
                    LinkText(text: "Company")
                }
                Text("Alex Kim · Engineering Manager").font(.ui(12.5)).foregroundStyle(ink)
                Text("Riley Chen · Former colleague").font(.ui(12.5)).foregroundStyle(ink)
            }
            .marker(4, x: -30)
        }
        .padding(Space.l)
    }

    private func point(_ symbol: String, _ tone: Tone, _ text: String, _ note: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 12)).foregroundStyle(tone.color)
            VStack(alignment: .leading, spacing: 2) {
                Text(text).font(.ui(12.5)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                Text(note).font(.ui(11)).foregroundStyle(secondaryInk)
            }
        }
    }
}

// MARK: - Overview proposed

/// The brief as a card: the verdict in a sentence, then what counts for and
/// against you in two short lists.
struct WhyItFitsCard: View {
    var twoColumns = true
    var body: some View {
        Card(title: "Why it fits", meta: "local model · 2 days ago", trailing: "Write full brief") {
            Text("Your payments and checkout cases map straight onto their rebuild, and contractor pay in USD likely clears your take-home target.")
                .font(.ui(13.5)).foregroundStyle(ink).lineSpacing(2).fixedSize(horizontal: false, vertical: true)
            let layout = twoColumns ? AnyLayout(HStackLayout(alignment: .top, spacing: Space.l)) : AnyLayout(VStackLayout(alignment: .leading, spacing: Space.m))
            layout {
                column("For you", .positive, "plus", [("Led a checkout rewrite that lifted conversion", "Case"), ("Five years of React and TypeScript", "Skill"), ("Remote with US teams today", "Experience")])
                column("Against", .caution, "minus", [("No production GraphQL federation", "Gap"), ("5 years in React; they ask 6+", "Screen")])
            }
            HStack(spacing: 5) {
                Image(systemName: "clock.arrow.circlepath").font(.system(size: 10))
                Text("Your knowledge base changed since. Write it again to refresh.").font(.ui(11)).fixedSize(horizontal: false, vertical: true)
            }
            .foregroundStyle(Tone.caution.color)
        }
    }

    private func column(_ title: String, _ tone: Tone, _ symbol: String, _ items: [(String, String)]) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title.uppercased()).font(.ui(10, .bold)).kerning(0.5).foregroundStyle(tone.color)
            ForEach(items, id: \.0) { item in
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    Image(systemName: symbol).font(.system(size: 9, weight: .heavy)).foregroundStyle(tone.color).frame(width: 10)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(item.0).font(.ui(12.5)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                        Text(item.1).font(.ui(10.5)).foregroundStyle(tertiaryInk)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }
}

/// The screen as a card: the checks that don't pass first, with their
/// quotes and a link into the posting; the ones that pass on one line.
struct ScreenCard: View {
    var body: some View {
        Card(title: "Screen", meta: "4 of 6 pass", trailing: "Criteria") {
            VStack(alignment: .leading, spacing: Space.s + 2) {
                exception("Years", "Asks 6+, you have 5 in React", "6+ years building web products with React")
                exception("Timezone", "4 h overlap with US Eastern, from Brasília", "At least 4 hours of overlap with US Eastern time.")
            }
            FlowLayout(spacing: 6) {
                ForEach(["Role", "Where they hire", "Stack", "Take-home"], id: \.self) { name in
                    HStack(spacing: 4) {
                        Image(systemName: "checkmark").font(.system(size: 9, weight: .heavy))
                        Text(name).font(.ui(11.5, .medium))
                    }
                    .foregroundStyle(Tone.positive.color)
                    .padding(.horizontal, 8).padding(.vertical, 3)
                    .background(Tone.positive.fill, in: Capsule())
                }
            }
        }
    }

    private func exception(_ name: String, _ reason: String, _ quote: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: "questionmark.circle.fill").font(.system(size: 12)).foregroundStyle(Tone.caution.color)
            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .firstTextBaseline) {
                    (Text(name).fontWeight(.semibold).foregroundColor(ink) + Text("  " + reason).foregroundColor(secondaryInk)).font(.ui(12.5))
                        .fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: 4)
                    LinkText(text: "In posting", symbol: "arrow.down.right")
                }
                Text("\u{201C}\(quote)\u{201D}").font(.ui(11.5)).italic().foregroundStyle(ink.opacity(0.8))
                    .padding(.horizontal, 6).padding(.vertical, 3)
                    .background(highlightCaution.opacity(0.7), in: RoundedRectangle(cornerRadius: 4))
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}

/// People at the company: a monogram, the name, how you know them, and the
/// action that fits.
struct PeopleCard: View {
    var body: some View {
        Card(title: "People", meta: "2 you know at Northwind", trailing: "All 6") {
            VStack(spacing: Space.s + 2) {
                person("AK", "Alex Kim", "Engineering Manager", "Connection", .accent, "Message")
                person("RC", "Riley Chen", "Former colleague · Staff Engineer", "Introducer", .positive, "Ask for intro")
            }
        }
    }

    private func person(_ initials: String, _ name: String, _ role: String, _ relation: String, _ tone: Tone, _ action: String) -> some View {
        HStack(spacing: Space.s + 2) {
            Text(initials).font(.ui(10.5, .bold)).foregroundStyle(secondaryInk)
                .frame(width: 28, height: 28).background(fill, in: Circle())
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 6) {
                    Text(name).font(.ui(12.5, .semibold)).foregroundStyle(ink)
                    Chip(text: relation, tone: tone)
                }
                Text(role).font(.ui(11.5)).foregroundStyle(secondaryInk).lineLimit(1)
            }
            Spacer(minLength: 0)
            SecondaryButton(title: action)
        }
    }
}

struct ProposedOverviewTab: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            WhyItFitsCard(twoColumns: true).marker(1, x: -30)
            ScreenCard().marker(2, x: -30)
            PeopleCard().marker(3, x: -30)
        }
        .padding(Space.l)
        .background(board)
    }
}

struct OverviewBeforeAfter: View {
    var body: some View {
        Board(
            eyebrow: "Page section · the job inspector",
            title: "The inspector says why, then what's in doubt",
            subtitle: "The first redesign gave the job one header, one primary action and tabs. What's left is the body: the Overview tab still reads as a document of equal-weight lists, and the header's two chips say less than the decision needs.",
            width: 1660
        ) {
            HStack(alignment: .top, spacing: 64) {
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Today", subtitle: "Overview tab: Brief, Screen, People, stacked.", tone: .negative)
                    PanelFrame(width: 460) {
                        VStack(alignment: .leading, spacing: 0) {
                            InspectorBar(expands: false)
                            CurrentJobTop(tab: "Overview")
                                .overlay(alignment: .topLeading) { Marker(number: 5).offset(x: -46, y: 92) }
                            CurrentOverviewTab()
                        }
                    }
                }
                .padding(.leading, 30)
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Proposed", subtitle: "Same width. A verdict strip, then three cards.", tone: .positive)
                    PanelFrame(width: 460) {
                        VStack(alignment: .leading, spacing: 0) {
                            InspectorBar()
                            ProposedJobTop(tab: "Overview")
                                .overlay(alignment: .topLeading) { Marker(number: 4).offset(x: -46, y: 92) }
                                .padding(.bottom, Space.xs)
                            ProposedOverviewTab()
                        }
                    }
                }
                .padding(.leading, 30)
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("Today").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "The verdict is a paragraph like any other", fix: "It leads its card at a reading size, with the model and the age on the title line.")
                    ProblemNote(number: 2, problem: "Strengths and weaknesses are two long lists, one under the other", fix: "For you and Against, side by side, a line each, with what backs it as a short tag.")
                    ProblemNote(number: 3, problem: "Six screen rows of equal weight bury the two that matter", fix: "What doesn't pass comes first, with its quote marked and a link into the posting. What passes is one row of checks.")
                    ProblemNote(number: 4, problem: "People are plain text with no next step", fix: "A person row: monogram, relation, role, and the action that fits: Message, Ask for intro.")
                    ProblemNote(number: 5, problem: "Two chips can't carry the decision", fix: "A verdict strip: Match, Screen, Take-home, People. Each cell opens its card.")
                    Divider().padding(.vertical, Space.s)
                    Text("Proposed").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Why it fits", fix: "The brief. Write full brief and the stale warning stay, smaller.")
                    ProblemNote(number: 2, problem: "Screen", fix: "4 of 6 pass. The exceptions, then the checks that pass.")
                    ProblemNote(number: 3, problem: "People", fix: "Who you know, and All 6 opens the company's People tab.")
                    ProblemNote(number: 4, problem: "Header", fix: "Company with its monogram and size, title, one line of facts, the verdict strip, actions with their keys, and Open posting as an icon.")
                }
                .frame(width: 460)
            }
        }
    }
}

// MARK: - The job as a page

/// A detail page's top: back to the list, where you are in it, and the
/// list's next and previous.
struct PageCrumbs: View {
    let list: String
    let title: String
    var position: String? = "3 of 48"
    var body: some View {
        HStack(spacing: Space.s) {
            HStack(spacing: 4) {
                Image(systemName: "chevron.backward").font(.system(size: 11, weight: .semibold))
                Text(list).font(.ui(12.5, .medium))
            }
            .foregroundStyle(Tone.accent.color)
            Text("/").font(.ui(12.5)).foregroundStyle(tertiaryInk)
            Text(title).font(.ui(12.5)).foregroundStyle(secondaryInk)
            Spacer()
            if let position {
                Text(position).font(.ui(11.5)).monospacedDigit().foregroundStyle(tertiaryInk)
                PlainIcon(symbol: "chevron.up")
                PlainIcon(symbol: "chevron.down")
            }
            PlainIcon(symbol: "arrow.down.right.and.arrow.up.left")
        }
        .padding(.horizontal, Space.l)
        .frame(height: 44)
        .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
    }
}

/// The job's header across a page: who and what on the left, the actions
/// on the right, and the verdict strip under it.
struct JobPageHeader: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .top, spacing: Space.m) {
                Monogram(letters: "N", hue: Color(hex: 0x0E7C86), size: 44)
                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 4) {
                        Text("Northwind").font(.ui(12.5, .medium)).foregroundStyle(Tone.accent.color)
                        Text("· Payments · 140 people").font(.ui(12.5)).foregroundStyle(secondaryInk)
                    }
                    Text(SampleJob.title).font(.ui(24, .bold)).foregroundStyle(ink)
                    Text("Remote, Americas · Contractor · Posted 2 days ago").font(.ui(12.5)).foregroundStyle(secondaryInk)
                }
                Spacer()
                HStack(spacing: Space.s) {
                    IconButton(symbol: "safari")
                    IconButton(symbol: "ellipsis")
                    SecondaryButton(title: "Skip…", shortcut: "S")
                    SecondaryButton(title: "Later", shortcut: "L")
                    PrimaryButton(title: "Pursue", symbol: "arrow.up.forward", shortcut: "P")
                }
            }
            VerdictStrip()
            TabStrip(tabs: [("Posting", nil), ("Session", "•")], selected: "Posting").padding(.top, Space.xs)
        }
    }
}

/// A job opened as a page: the posting in a reading column with its outline
/// beside it, and the decision's cards on a rail.
struct JobPage: View {
    var showsOutline = true
    var railWidth: CGFloat = 340
    var measure: CGFloat = 600
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            JobPageHeader().padding(.horizontal, Space.xl).padding(.top, Space.l).padding(.bottom, 0)
            HStack(alignment: .top, spacing: Space.xl) {
                if showsOutline {
                    Outline(vertical: true).frame(width: 150, alignment: .leading).padding(.top, 4)
                }
                VStack(alignment: .leading, spacing: Space.l) {
                    HStack(spacing: 6) {
                        Image(systemName: "doc.plaintext").font(.system(size: 11)).foregroundStyle(secondaryInk)
                        Text("From Northwind's Greenhouse board · read 4 Oct").font(.ui(11.5)).foregroundStyle(secondaryInk)
                        Spacer()
                        LinkText(text: "Original", symbol: "arrow.up.right")
                    }
                    KeyFacts(columns: 3)
                        .padding(Space.m + 2)
                        .background(well, in: RoundedRectangle(cornerRadius: Radius.card))
                    HighlightLegend()
                    PostingReader(size: 14)
                }
                .frame(width: measure)
                VStack(alignment: .leading, spacing: Space.m) {
                    WhyItFitsCard(twoColumns: false)
                    ScreenCard()
                    PeopleCard()
                }
                .frame(width: railWidth)
            }
            .padding(.horizontal, Space.xl)
            .padding(.top, Space.l)
        }
    }
}

struct JobPageWindow: View {
    var body: some View {
        Board(
            eyebrow: "Pattern · list, inspector, page",
            title: "A job opens as a page when you want to read it",
            subtitle: "The inspector is for triage beside a list. Return, a double-click or the expand button opens the job as a page in the content column: the posting at a reading width, its outline, and the decision's cards on a rail. ↑↓ go to the list's next and previous; Esc or ⌘[ goes back to the list where you were.",
            width: 1520
        ) {
            WindowFrame(width: 1424, height: 1270) {
                HStack(spacing: 0) {
                    Sidebar(selected: "Jobs", proposed: true)
                    VStack(spacing: 0) {
                        PageCrumbs(list: "Jobs", title: SampleJob.title)
                        JobPage().withoutMarkers()
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                    .background(surface)
                }
            }
        }
    }
}
