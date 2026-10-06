// User stories with their screen journeys, and the decisions the proposal
// rests on, with the options weighed.
import SwiftUI

// MARK: - Journey pieces

struct Story: View {
    let number: Int
    let who: String
    let want: String
    let soThat: String
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text("STORY \(number)").font(.ui(10.5, .bold)).kerning(0.6).foregroundStyle(Tone.accent.color)
            (Text("As ").foregroundColor(secondaryInk) + Text(who).foregroundColor(ink).fontWeight(.semibold)
                + Text(", I want ").foregroundColor(secondaryInk) + Text(want).foregroundColor(ink).fontWeight(.semibold)
                + Text(", so ").foregroundColor(secondaryInk) + Text(soThat).foregroundColor(ink))
                .font(.ui(13.5)).lineSpacing(2).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Space.l)
        .frame(width: 290, alignment: .topLeading)
        .frame(maxHeight: .infinity, alignment: .topLeading)
        .background(Tone.accent.color.opacity(0.06), in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(Tone.accent.color.opacity(0.18)))
    }
}

struct Step<Content: View>: View {
    let number: Int
    let caption: String
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            content
                .frame(width: 290, height: 200)
                .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
                .clipShape(RoundedRectangle(cornerRadius: Radius.card))
                .shadow(color: .black.opacity(0.06), radius: 6, y: 3)
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text("\(number)").font(.ui(11, .bold)).foregroundStyle(.white).frame(width: 18, height: 18).background(ink, in: Circle())
                Text(caption).font(.ui(12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
            .frame(width: 290, alignment: .leading)
        }
    }
}

struct StepArrow: View {
    let action: String
    var body: some View {
        VStack(spacing: 4) {
            Text(action).font(.ui(11, .semibold)).foregroundStyle(Tone.accent.color).multilineTextAlignment(.center).frame(width: 56)
            Image(systemName: "arrow.right").font(.system(size: 14, weight: .semibold)).foregroundStyle(Tone.accent.color)
        }
        .frame(width: 60)
        .padding(.top, 80)
    }
}

/// A tap or a key, drawn over a step where it happens.
struct Tap: View {
    var body: some View {
        Circle().fill(annotation.opacity(0.25)).frame(width: 26, height: 26)
            .overlay(Circle().strokeBorder(annotation, lineWidth: 2))
    }
}

struct Toast: View {
    let text: String
    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "checkmark.circle.fill").font(.system(size: 12)).foregroundStyle(Tone.positive.color)
            Text(text).font(.ui(11.5, .medium)).foregroundStyle(.white).lineLimit(1)
            Text("Undo").font(.ui(11.5, .semibold)).foregroundStyle(Color(hex: 0xB9B8FF))
        }
        .padding(.horizontal, 12).padding(.vertical, 7)
        .background(Color(hex: 0x2C2C30), in: Capsule())
        .shadow(color: .black.opacity(0.2), radius: 6, y: 3)
    }
}

// MARK: - Journeys

struct JourneysBoard: View {
    var body: some View {
        Board(
            eyebrow: "User stories and screen journeys",
            title: "Four things you come to the app to do",
            subtitle: "Each story follows one person, the owner of the hub, through the proposed screens, step by step. Arrows name what they press or click. Every step is something the sub-tickets deliver.",
            width: 1860
        ) {
            VStack(alignment: .leading, spacing: 44) {
                journey(Story(number: 1, who: "someone with seven jobs to decide", want: "to read each posting with what the hub found marked in it", soThat: "I decide in a minute without opening the company's site")) {
                    Step(number: 1, caption: "Today's Decide card lists the best matches. Return on one opens Decide on it.") {
                        Scaled(scale: 0.7, width: 414, height: 286) {
                            Card(title: "Decide", trailing: "All 7") {
                                VStack(alignment: .leading, spacing: 10) {
                                    decideRow("Strong", .positive, "Senior Product Engineer", "Northwind · Remote, Americas")
                                    decideRow("Possible", .accent, "Staff Frontend Engineer", "Globex · Remote, LATAM")
                                    decideRow("Stretch", .caution, "Senior Software Engineer, Platform", "Umbrella Labs · Remote")
                                }
                            }
                            .padding(Space.m)
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 30, y: 36) }
                    }
                    StepArrow(action: "Return")
                    Step(number: 2, caption: "Decide shows the queue beside the job's page: key facts, the posting, the cards that judge it.") {
                        Scaled(scale: 0.29, width: 1000, height: 690) {
                            JobPage(showsOutline: false, railWidth: 330, measure: 560).withoutMarkers()
                        }
                    }
                    StepArrow(action: "In posting")
                    Step(number: 3, caption: "Screen › Years › In posting scrolls the posting to the line it quoted, marked.") {
                        Scaled(scale: 0.72, width: 403, height: 278) {
                            VStack(alignment: .leading, spacing: 6) {
                                Text("What you bring").font(.ui(15, .semibold)).foregroundStyle(ink)
                                ForEach(["[[6+ years building web products with React]] and TypeScript.", "Production experience with Node.js and PostgreSQL.", "You own features without waiting for a spec."], id: \.self) { line in
                                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                                        Text("•").foregroundStyle(tertiaryInk)
                                        SampleJob.styled(line, size: 13).font(.ui(13)).foregroundStyle(ink)
                                    }
                                }
                                MarginNote(symbol: "questionmark.circle.fill", tone: .caution, title: "Screen · Years", text: "Asks 6+, you have 5 in React", width: 330)
                            }
                            .padding(Space.l)
                        }
                    }
                    StepArrow(action: "P")
                    Step(number: 4, caption: "P pursues it. The next job opens at once, with Undo for a moment.") {
                        ZStack(alignment: .bottom) {
                            Scaled(scale: 0.72, width: 403, height: 278) {
                                VStack(alignment: .leading, spacing: 4) {
                                    queueRow("Strong", .positive, "Full-Stack Engineer, Payments", "Initech", selected: true)
                                    queueRow("Possible", .accent, "Staff Frontend Engineer", "Globex")
                                    queueRow("Possible", .accent, "Frontend Engineer II", "Brightline")
                                }
                                .padding(Space.m)
                            }
                            Toast(text: "Pursued Senior Product Engineer").padding(.bottom, 14)
                        }
                    }
                }
                journey(Story(number: 2, who: "someone looking over 48 open jobs", want: "each row to say the match, the screen and the pay", soThat: "I open only the jobs worth reading")) {
                    Step(number: 1, caption: "Jobs: the filters that are on show as chips. Remote is one click off.") {
                        Scaled(scale: 0.5, width: 580, height: 400) {
                            VStack(spacing: 0) {
                                PageHeader(scopes: [("Open", "48"), ("Later", "6"), ("Skipped", "112")], selectedScope: "Open", filters: ["Passes screen", "Remote"], search: "Search jobs").padding(.top, Space.l)
                                ProposedJobsTable(rowCount: 5)
                            }
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 68, y: 17) }
                    }
                    StepArrow(action: "Hover")
                    Step(number: 2, caption: "A row reads Mismatch and fails Where. Hovering it shows Pursue, Later and Skip.") {
                        Scaled(scale: 0.48, width: 600, height: 410) {
                            VStack(spacing: 0) {
                                ForEach(SampleJobs.rows.dropFirst(4).prefix(5)) { row in ProposedJobRow(row: row, isHovered: row.id == 6) }
                            }
                            .padding(Space.m)
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 100, y: 45) }
                    }
                    StepArrow(action: "S")
                    Step(number: 3, caption: "S skips it with no sheet in the way; the reason can be added from Undo's toast.") {
                        ZStack(alignment: .bottom) {
                            Scaled(scale: 0.48, width: 600, height: 410) {
                                VStack(spacing: 0) {
                                    ForEach(SampleJobs.rows.dropFirst(4).prefix(5).filter { $0.id != 6 }) { row in ProposedJobRow(row: row) }
                                }
                                .padding(Space.m)
                            }
                            Toast(text: "Skipped Product Engineer").padding(.bottom, 14)
                        }
                    }
                    StepArrow(action: "Return")
                    Step(number: 4, caption: "Return on Initech's row opens it as a page; Esc goes back to the row.") {
                        Scaled(scale: 0.24, width: 1200, height: 830) {
                            VStack(spacing: 0) {
                                PageCrumbs(list: "Jobs", title: "Full-Stack Engineer, Payments")
                                JobPage().withoutMarkers()
                            }
                            .background(surface)
                        }
                    }
                }
                journey(Story(number: 3, who: "someone with applications out", want: "the app to say which follow-ups are due and who to write to", soThat: "nothing goes quiet without me noticing")) {
                    Step(number: 1, caption: "Pipeline's badge is red: a follow-up is overdue. Grey counts never ask.") {
                        Scaled(scale: 0.9, width: 322, height: 222) {
                            VStack(alignment: .leading, spacing: 1) {
                                SidebarItem(title: "Today", symbol: "sun.max", badge: 4)
                                SidebarItem(title: "Decide", symbol: "checklist", badge: 7)
                                SidebarItem(title: "Pipeline", symbol: "rectangle.split.3x1", badge: 1, badgeTone: .negative)
                                SidebarHeader(title: "Browse")
                                SidebarItem(title: "Jobs", symbol: "briefcase")
                            }
                            .padding(Space.l)
                            .frame(width: 260)
                            .background(sidebarBackground)
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 189, y: 60) }
                    }
                    StepArrow(action: "Click")
                    Step(number: 2, caption: "The Due scope shows the cards to act on, the overdue one on top, edged in red.") {
                        Scaled(scale: 0.78, width: 372, height: 256) {
                            VStack(alignment: .leading, spacing: Space.s) {
                                HStack(spacing: 6) {
                                    Text("Applied").font(.ui(12.5, .semibold))
                                    Chip(text: "2 due", tone: .negative, symbol: "bell.fill")
                                }
                                ProposedCard(card: SampleBoard.phases[0].1[0])
                                ProposedCard(card: SampleBoard.phases[0].1[1])
                            }
                            .padding(Space.l)
                            .frame(width: 300)
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 57, y: 88) }
                    }
                    StepArrow(action: "Write to Alex Kim")
                    Step(number: 3, caption: "Write to Alex Kim drafts the note in the job's session. ✓ records the follow-up.") {
                        Scaled(scale: 0.78, width: 372, height: 256) {
                            VStack(alignment: .leading, spacing: Space.m) {
                                Text("Followed up").font(.ui(14, .semibold)).foregroundStyle(ink)
                                Text("What you did").font(.ui(11.5)).foregroundStyle(secondaryInk)
                                Text("Wrote to Alex Kim on LinkedIn").font(.ui(12.5)).foregroundStyle(ink)
                                    .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                                    .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(Tone.accent.color, lineWidth: 1.5))
                                HStack {
                                    Text("Next follow-up in 7 days").font(.ui(11.5)).foregroundStyle(secondaryInk)
                                    Spacer()
                                    PrimaryButton(title: "Record")
                                }
                            }
                            .padding(Space.l)
                            .frame(width: 320)
                            .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
                            .shadow(color: .black.opacity(0.15), radius: 10, y: 4)
                            .padding(Space.l)
                        }
                    }
                    StepArrow(action: "Record")
                    Step(number: 4, caption: "The card's status goes quiet: Follow up in 7 days. The badge clears.") {
                        Scaled(scale: 0.78, width: 372, height: 256) {
                            VStack(alignment: .leading, spacing: Space.s) {
                                HStack(spacing: 6) {
                                    Text("Applied").font(.ui(12.5, .semibold))
                                    Chip(text: "1 due", tone: .caution, symbol: "bell.fill")
                                }
                                ProposedCard(card: CardData(id: 9, title: "Full-Stack Engineer, Payments", company: "Initech", monogram: "I", hue: 0x2563EB, followUp: ("In 7 days", .neutral), days: 12))
                                ProposedCard(card: SampleBoard.phases[0].1[1])
                            }
                            .padding(Space.l)
                            .frame(width: 300)
                        }
                    }
                }
                journey(Story(number: 4, who: "someone whose search changed", want: "to edit my criteria as lists of places and words", soThat: "every page screens jobs by what I want now")) {
                    Step(number: 1, caption: "Criteria › Search. EU only no longer rules you out: its × takes it off.") {
                        Scaled(scale: 0.66, width: 440, height: 303) {
                            VStack(alignment: .leading, spacing: Space.m) {
                                Text("Where you can work").font(.ui(14, .semibold))
                                tokenRow("Hires from", ["Brazil", "LATAM", "Americas", "worldwide"], .positive)
                                tokenRow("Rules me out", ["must reside in the US", "US only", "EU only"], .negative)
                            }
                            .padding(Space.l)
                        }
                        .overlay(alignment: .topLeading) { Tap().offset(x: 182, y: 76) }
                    }
                    StepArrow(action: "×, then type")
                    Step(number: 2, caption: "Typing Europe and Return adds it to Hires from as a token.") {
                        Scaled(scale: 0.66, width: 440, height: 303) {
                            VStack(alignment: .leading, spacing: Space.m) {
                                Text("Where you can work").font(.ui(14, .semibold))
                                tokenRow("Hires from", ["Brazil", "LATAM", "Americas", "worldwide", "Europe"], .positive)
                                tokenRow("Rules me out", ["must reside in the US", "US only"], .negative)
                            }
                            .padding(Space.l)
                        }
                    }
                    StepArrow(action: "⌘S")
                    Step(number: 3, caption: "The save bar counts the changes; ⌘S or Save keeps them.") {
                        ZStack(alignment: .bottom) {
                            Scaled(scale: 0.66, width: 440, height: 303) {
                                VStack(alignment: .leading, spacing: Space.m) {
                                    Text("Where you can work").font(.ui(14, .semibold))
                                    tokenRow("Hires from", ["Brazil", "LATAM", "Americas", "worldwide", "Europe"], .positive)
                                }
                                .padding(Space.l)
                            }
                            HStack(spacing: 8) {
                                Circle().fill(Tone.accent.color).frame(width: 6, height: 6)
                                Text("2 unsaved changes").font(.ui(11, .medium))
                                Spacer()
                                PrimaryButton(title: "Save", shortcut: "⌘S").scaleEffect(0.85)
                            }
                            .padding(.horizontal, 12).padding(.vertical, 6)
                            .frame(width: 250)
                            .background(surface, in: Capsule())
                            .overlay(Capsule().strokeBorder(separator))
                            .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
                            .padding(.bottom, 14)
                        }
                    }
                    StepArrow(action: "Saved")
                    Step(number: 4, caption: "Every page reads the new criteria: a job hiring in Europe that failed Where now passes.") {
                        ZStack(alignment: .bottom) {
                            Scaled(scale: 0.48, width: 600, height: 410) {
                                VStack(spacing: 0) {
                                    ProposedJobRow(row: JobRowData(id: 20, title: "Senior Product Engineer", company: "Hearth", monogram: "H", hue: 0xBE185D, location: "Remote, Europe", match: ("Possible", .accent), screen: ("Passes", .positive, "checkmark.circle.fill"), takeHome: "R$ 34k", posted: "3 d"), isSelected: true)
                                    ForEach(SampleJobs.rows.prefix(3)) { row in ProposedJobRow(row: row) }
                                }
                                .padding(Space.m)
                            }
                            Toast(text: "Saved your criteria").padding(.bottom, 14)
                        }
                    }
                }
            }
        }
    }

    private func journey<Steps: View>(_ story: Story, @ViewBuilder steps: () -> Steps) -> some View {
        HStack(alignment: .top, spacing: Space.xl) {
            story
            HStack(alignment: .top, spacing: 4) { steps() }
        }
        .fixedSize(horizontal: false, vertical: true)
    }

    private func decideRow(_ match: String, _ tone: Tone, _ title: String, _ detail: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 6) {
                Chip(text: match, tone: tone)
                Text(title).font(.ui(12.5, .semibold)).foregroundStyle(ink)
            }
            Text(detail).font(.ui(11.5)).foregroundStyle(secondaryInk)
        }
    }

    private func queueRow(_ match: String, _ tone: Tone, _ title: String, _ company: String, selected: Bool = false) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 5) {
                Text(match).font(.ui(11, .semibold)).foregroundStyle(tone.color)
                Text("· \(company)").font(.ui(11.5)).foregroundStyle(secondaryInk)
            }
            Text(title).font(.ui(13, .semibold)).foregroundStyle(ink)
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(selected ? Tone.accent.color.opacity(0.10) : .clear, in: RoundedRectangle(cornerRadius: 8))
    }

    private func tokenRow(_ label: String, _ tokens: [String], _ tone: Tone) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label).font(.ui(11.5)).foregroundStyle(secondaryInk)
            TokenField(tokens: tokens, tone: tone)
        }
    }
}

// MARK: - Decisions

struct DecisionCard: View {
    let question: String
    let options: [(String, String, Bool)]
    let because: String
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text(question).font(.ui(15, .semibold)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: Space.s) {
                ForEach(options, id: \.0) { option in
                    HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                        Image(systemName: option.2 ? "checkmark.circle.fill" : "circle")
                            .font(.system(size: 12)).foregroundStyle(option.2 ? Tone.positive.color : tertiaryInk)
                        VStack(alignment: .leading, spacing: 1) {
                            Text(option.0).font(.ui(12.5, option.2 ? .semibold : .regular)).foregroundStyle(option.2 ? ink : secondaryInk)
                            Text(option.1).font(.ui(11.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
                        }
                    }
                    .padding(10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(option.2 ? Tone.positive.fill : fillSubtle, in: RoundedRectangle(cornerRadius: 8))
                }
            }
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text("Because").font(.ui(11.5, .semibold)).foregroundStyle(Tone.accent.color)
                Text(because).font(.ui(12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(Space.l + 2)
        .frame(width: 400, alignment: .topLeading)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
    }
}

struct DecisionsBoard: View {
    var body: some View {
        Board(
            eyebrow: "Decisions",
            title: "Four choices the proposal makes, and what they were weighed against",
            subtitle: "Each is proposed, for review on TP-659. The doc's Decisions section records them the same way.",
            width: 1760
        ) {
            HStack(alignment: .top, spacing: Space.l) {
                DecisionCard(
                    question: "How does the posting keep its structure?",
                    options: [
                        ("Keep HTML; the app draws it", "AttributedString(html:) or a web view. Heavy, slow in a list, unsafe markup, and Android would need its own.", false),
                        ("Markdown on the server", "ConvertHTMLToMarkdown beside ConvertHTMLToText. One small converter; both clients and the models read it.", true),
                        ("Guess structure from plain text", "Lines ending in a colon become headings. Breaks on half the boards.", false),
                    ],
                    because: "the server already owns every board's parsing, and Markdown is the format the app's MarkdownBlocks and the prompts already read."
                )
                DecisionCard(
                    question: "Where do you read a job?",
                    options: [
                        ("Only the inspector, wider", "720 pt at most, beside a list. Still a sidebar's measure, and squeezes the list.", false),
                        ("A window per job", "Like sessions. Windows pile up, and the list's ↑↓ can't drive them.", false),
                        ("Inspector to triage, page to read", "Return opens the job as a page in the content column, with the list's ↑↓ and Esc back.", true),
                    ],
                    because: "triage and reading need different widths, and a page keeps the list one key away."
                )
                DecisionCard(
                    question: "Where do a page's controls live?",
                    options: [
                        ("The window toolbar", "The macOS default. Reaches over the inspector, and has looped AppKit's constraints (#66, #71).", false),
                        ("A bar of icons over the page", "Today's PageBar. Works, but hides what's on and fits no scopes.", false),
                        ("A page header in the content", "Scopes, search, Add, then filter chips. The toolbar keeps only the title.", true),
                    ],
                    because: "it's the PageBar's safe spot, with room to say what's on."
                )
                DecisionCard(
                    question: "What does Decide look like?",
                    options: [
                        ("List and inspector, as today", "The posting is a tab away at 420 pt.", false),
                        ("One job full screen, no list", "Focused, but you lose where you are in the queue.", false),
                        ("A narrow queue beside the job page", "The queue at 300 pt, the posting and its cards beside it.", true),
                    ],
                    because: "deciding is reading; Decide is the one page where every job needs the posting."
                )
            }
        }
    }
}
