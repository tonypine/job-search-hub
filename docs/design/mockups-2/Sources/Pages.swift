// Pages: Pipeline and Companies before and after, and Decide as a queue
// beside the job's page.
import SwiftUI

// MARK: - Pipeline

struct CardData: Identifiable {
    let id: Int
    let title: String
    let company: String
    let monogram: String
    let hue: UInt32
    var heardBack: String?
    var followUp: (String, Tone)?
    var secondRoute: String?
    var closed: String?
    var days: Int
}

enum SampleBoard {
    static let phases: [(String, [CardData])] = [
        ("Applied", [
            CardData(id: 0, title: "Full-Stack Engineer, Payments", company: "Initech", monogram: "I", hue: 0x2563EB, followUp: ("Overdue 2 days", .negative), secondRoute: "Alex Kim", days: 12),
            CardData(id: 1, title: "Frontend Engineer II", company: "Brightline", monogram: "B", hue: 0x0F766E, followUp: ("Due today", .caution), days: 7),
            CardData(id: 2, title: "Senior Frontend Engineer", company: "Vandelay", monogram: "V", hue: 0x4D7C0F, followUp: ("In 3 days", .neutral), days: 4),
        ]),
        ("Screening", [
            CardData(id: 3, title: "Staff Frontend Engineer", company: "Globex", monogram: "G", hue: 0x7A4FD1, heardBack: "2 Oct", followUp: ("In 5 days", .neutral), days: 3),
        ]),
        ("Interviewing", [
            CardData(id: 4, title: "Senior Software Engineer, Platform", company: "Umbrella Labs", monogram: "U", hue: 0xB91C1C, heardBack: "28 Sep", days: 9),
            CardData(id: 5, title: "Senior Engineer, Growth", company: "Lumen Health", monogram: "L", hue: 0xC2410C, heardBack: "1 Oct", followUp: ("Due today", .caution), days: 5),
        ]),
        ("Offer", []),
        ("Closed", [
            CardData(id: 6, title: "Product Engineer", company: "Acme Robotics", monogram: "A", hue: 0x9333EA, closed: "Role filled internally", days: 20),
            CardData(id: 7, title: "Senior React Engineer", company: "Cobalt", monogram: "C", hue: 0x1D4ED8, closed: "Needed US residency", days: 31),
        ]),
    ]
}

/// A Pipeline card as built: up to seven lines, each in its own color.
struct CurrentCard: View {
    let card: CardData
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            Text(card.title).font(.ui(12.5, .medium)).foregroundStyle(ink).lineLimit(2).fixedSize(horizontal: false, vertical: true)
            Text(card.company).font(.ui(12.5)).foregroundStyle(secondaryInk)
            if let heardBack = card.heardBack {
                Label("Heard back \(heardBack)", systemImage: "arrowshape.turn.up.left").font(.ui(11)).foregroundStyle(Tone.positive.color)
            }
            if let followUp = card.followUp { Chip(text: followUp.0, tone: followUp.1) }
            if let route = card.secondRoute {
                VStack(alignment: .leading, spacing: 4) {
                    Text("No answer. Try someone else:").font(.ui(11)).foregroundStyle(secondaryInk)
                    HStack(spacing: 4) {
                        SecondaryButton(title: "Write to \(route)", symbol: "paperplane").scaleEffect(0.9, anchor: .leading)
                    }
                    SecondaryButton(title: "Followed up…").scaleEffect(0.9, anchor: .leading)
                }
            }
            if let closed = card.closed {
                Label(closed, systemImage: "archivebox").font(.ui(11)).foregroundStyle(secondaryInk).lineLimit(2)
            }
            Text(card.days == 1 ? "1 day in phase" : "\(card.days) days in phase").font(.ui(11)).foregroundStyle(secondaryInk)
        }
        .padding(Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(card.id == 0 ? Tone.accent.color : separator, lineWidth: card.id == 0 ? 2 : 1))
    }
}

/// A Pipeline card as proposed: the company, the job, and one status line,
/// the most urgent; a next step only where there's one to take.
struct ProposedCard: View {
    let card: CardData
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Monogram(letters: card.monogram, hue: Color(hex: card.hue), size: 18)
                Text(card.company).font(.ui(11.5, .medium)).foregroundStyle(secondaryInk)
                Spacer(minLength: 0)
                Text("\(card.days) d").font(.ui(11)).monospacedDigit().foregroundStyle(tertiaryInk)
            }
            Text(card.title).font(.ui(13, .semibold)).foregroundStyle(ink).lineLimit(2).fixedSize(horizontal: false, vertical: true)
            status
            if let route = card.secondRoute {
                HStack(spacing: 6) {
                    Image(systemName: "paperplane").font(.system(size: 10))
                    Text("Write to \(route)").font(.ui(11.5, .medium))
                    Spacer(minLength: 0)
                    Image(systemName: "checkmark").font(.system(size: 10, weight: .bold))
                        .frame(width: 22, height: 20).background(fill, in: RoundedRectangle(cornerRadius: 5))
                        .foregroundStyle(ink)
                }
                .foregroundStyle(Tone.accent.color)
                .padding(.top, 2)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(alignment: .leading) {
            if let followUp = card.followUp, followUp.1 != .neutral {
                UnevenRoundedRectangle(topLeadingRadius: Radius.card, bottomLeadingRadius: Radius.card).fill(followUp.1.color).frame(width: 3)
            }
        }
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(card.id == 0 ? Tone.accent.color : separator, lineWidth: card.id == 0 ? 2 : 1))
        .shadow(color: .black.opacity(0.04), radius: 2, y: 1)
    }

    @ViewBuilder
    private var status: some View {
        if let followUp = card.followUp, followUp.1 != .neutral {
            HStack(spacing: 4) {
                Image(systemName: "bell.fill").font(.system(size: 9.5))
                Text("Follow-up \(followUp.0.lowercased())").font(.ui(11.5, .medium))
            }
            .foregroundStyle(followUp.1.color)
        } else if let heardBack = card.heardBack {
            HStack(spacing: 4) {
                Image(systemName: "arrowshape.turn.up.left.fill").font(.system(size: 9.5))
                Text("Heard back \(heardBack)").font(.ui(11.5))
            }
            .foregroundStyle(Tone.positive.color)
        } else if let followUp = card.followUp {
            Text("Follow up \(followUp.0.lowercased())").font(.ui(11.5)).foregroundStyle(secondaryInk)
        }
    }
}

struct CurrentPipelinePage: View {
    var body: some View {
        VStack(spacing: 0) {
            TitleBar(title: "Pipeline", subtitle: "6 applications · 4 contacted this week")
            HStack(spacing: Space.s) {
                Spacer()
                toggle("Due only", "bell.badge")
                toggle("Skipped", "eye.slash")
            }
            .padding(.horizontal, Space.m).padding(.vertical, Space.s)
            .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
            .marker(1, .leading, x: 12)
            HStack(alignment: .top, spacing: Space.m) {
                ForEach(SampleBoard.phases, id: \.0) { phase in
                    VStack(alignment: .leading, spacing: Space.s) {
                        HStack {
                            Text(phase.0).font(.ui(13, .semibold)).foregroundStyle(ink)
                            Spacer()
                            Text("\(phase.1.count)").font(.ui(12)).foregroundStyle(secondaryInk)
                        }
                        ForEach(phase.1) { CurrentCard(card: $0) }
                        Spacer(minLength: 0)
                    }
                    .padding(Space.s)
                    .frame(width: 196)
                    .frame(maxHeight: .infinity, alignment: .top)
                    .background(well, in: RoundedRectangle(cornerRadius: Radius.card))
                }
            }
            .padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(alignment: .topLeading) { Marker(number: 2).offset(x: 160, y: 60) }
            .overlay(alignment: .topLeading) { Marker(number: 3).offset(x: 160, y: 140) }
            .overlay(alignment: .topTrailing) { Marker(number: 4).offset(x: -10, y: 40) }
        }
        .background(surface)
    }

    private func toggle(_ title: String, _ symbol: String) -> some View {
        HStack(spacing: 4) {
            Image(systemName: symbol).font(.system(size: 11))
            Text(title).font(.ui(12))
        }
        .padding(.horizontal, 9).padding(.vertical, 4).background(fill, in: RoundedRectangle(cornerRadius: 6)).foregroundStyle(ink)
    }
}

struct ProposedPipelinePage: View {
    var body: some View {
        VStack(spacing: 0) {
            TitleBar(title: "Pipeline", subtitle: "6 active · 1 overdue · 2 due today")
            PageHeader(scopes: [("Active", "6"), ("Due", "3"), ("Closed", "9"), ("Skipped", "4")], selectedScope: "Active", filters: [], addFilter: false, search: "Search applications", addTitle: "Add")
                .marker(1, .topLeading, x: -4, y: -18)
            HStack(alignment: .top, spacing: Space.m) {
                ForEach(SampleBoard.phases.dropLast(), id: \.0) { phase in
                    VStack(alignment: .leading, spacing: Space.s) {
                        HStack(spacing: 6) {
                            Text(phase.0).font(.ui(12.5, .semibold)).foregroundStyle(ink)
                            Text("\(phase.1.count)").font(.ui(12)).foregroundStyle(tertiaryInk)
                            Spacer()
                            let due = phase.1.filter { ($0.followUp?.1 ?? .neutral) != .neutral }.count
                            if due > 0 { Chip(text: "\(due) due", tone: phase.1.contains { $0.followUp?.1 == .negative } ? .negative : .caution, symbol: "bell.fill") }
                        }
                        .padding(.horizontal, 4)
                        if phase.1.isEmpty {
                            Text("Drop a card here when an offer comes in")
                                .font(.ui(11.5)).foregroundStyle(tertiaryInk).multilineTextAlignment(.center)
                                .frame(maxWidth: .infinity).padding(.vertical, Space.l)
                                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator, style: StrokeStyle(lineWidth: 1, dash: [4, 3])))
                        }
                        ForEach(phase.1) { ProposedCard(card: $0) }
                        Spacer(minLength: 0)
                    }
                    .frame(width: 214)
                    .frame(maxHeight: .infinity, alignment: .top)
                }
                // Closed folds into a narrow drawer at the board's end.
                VStack(spacing: Space.s) {
                    Image(systemName: "archivebox").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    Text("Closed").font(.ui(12, .semibold)).foregroundStyle(secondaryInk).fixedSize().rotationEffect(.degrees(90)).frame(width: 20, height: 50)
                    Text("9").font(.ui(11)).foregroundStyle(tertiaryInk)
                    Spacer()
                }
                .padding(.vertical, Space.m)
                .frame(width: 40)
                .frame(maxHeight: .infinity)
                .background(well, in: RoundedRectangle(cornerRadius: Radius.card))
                .marker(4, .top, x: 0, y: -8)
            }
            .padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(board)
            .overlay(alignment: .topLeading) { Marker(number: 2).offset(x: 6, y: 74) }
            .overlay(alignment: .topLeading) { Marker(number: 3).offset(x: 6, y: 160) }
        }
        .background(surface)
    }
}

struct PipelineBeforeAfter: View {
    var body: some View {
        Board(
            eyebrow: "Page · Pipeline",
            title: "A card says one thing: what's next",
            subtitle: "A card today can carry seven lines in five colors, and the second route puts two buttons in it. The proposal keeps three lines and one status, the most urgent, and shows the next step only on the card that has one.",
            width: 1840
        ) {
            HStack(alignment: .top, spacing: 48) {
                VStack(alignment: .leading, spacing: Space.xxl) {
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Today", subtitle: "Five phase columns, Closed among them.", tone: .negative)
                        WindowFrame(width: 1160, height: 600) { CurrentPipelinePage() }
                    }
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Proposed", subtitle: "The same applications.", tone: .positive)
                        WindowFrame(width: 1160, height: 600) { ProposedPipelinePage() }
                    }
                }
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("Today").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Due only and Skipped are toggles on a right-hand bar", fix: "Scopes in the page header: Active, Due, Closed, Skipped, with counts.")
                    ProblemNote(number: 2, problem: "Heard back, follow-up, closed reason and days in phase stack up in different colors", fix: "One status line, by priority: overdue, due today, heard back, next follow-up. The age moves to the corner.")
                    ProblemNote(number: 3, problem: "The second route adds a sentence and two buttons to the card", fix: "One link, Write to Alex Kim, and a ✓ for Followed up. The choice of who goes in its menu.")
                    ProblemNote(number: 4, problem: "Closed takes a full column of finished cards", fix: "It folds into a drawer at the end; the Closed scope lists them.")
                    Divider().padding(.vertical, Space.s)
                    Text("Proposed").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Page header", fix: "The same header as every page. Add is an application by hand.")
                    ProblemNote(number: 2, problem: "Column header", fix: "The phase, its count, and a chip when something in it is due.")
                    ProblemNote(number: 3, problem: "Card", fix: "Company with its monogram and age, the job, one status. An edge in the status's tone when it's due.")
                    ProblemNote(number: 4, problem: "Closed drawer", fix: "Drop a card on it to close it, with the reason asked as today.")
                }
                .frame(width: 440)
            }
        }
    }
}

// MARK: - Companies

struct CompaniesBeforeAfter: View {
    private let companies: [(String, String, UInt32, String, String, String?, (String, Tone)?, Int, String, String?)] = [
        ("Northwind", "N", 0x0E7C86, "Payments · 140 people", "Greenhouse", "3 open", ("Strong", .positive), 2, "Today", nil),
        ("Globex", "G", 0x7A4FD1, "Retail tech · 900 people", "Lever", "5 open", ("Possible", .accent), 1, "Screening", nil),
        ("Initech", "I", 0x2563EB, "Fintech · 60 people", "Ashby", "1 open", ("Strong", .positive), 0, "Applied", nil),
        ("Umbrella Labs", "U", 0xB91C1C, "Health data · 300 people", "Workable", "2 open", ("Stretch", .caution), 3, "Interviewing", nil),
        ("Brightline", "B", 0x0F766E, "Logistics · 220 people", "No board found", nil, nil, 0, "—", "Find board"),
        ("Lumen Health", "L", 0xC2410C, "Researching…", "", nil, nil, 0, "", "research"),
    ]

    var body: some View {
        Board(
            eyebrow: "Page · Companies",
            title: "A company row answers: is there something for me here?",
            subtitle: "Today's table lists what the hub stored: domain, watched since, the board's address, a count of people. The proposal lists what you'd decide by: open jobs and the best match among them, who you know, where your application stands.",
            width: 1840
        ) {
            HStack(alignment: .top, spacing: 48) {
                VStack(alignment: .leading, spacing: Space.xxl) {
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Today", subtitle: "Seven columns of stored fields.", tone: .negative)
                        PanelFrame(width: 1160) { currentTable }
                    }
                    VStack(alignment: .leading, spacing: Space.m) {
                        ColumnHeading(title: "Proposed", subtitle: "The page header, and rows to decide by.", tone: .positive)
                        PanelFrame(width: 1160) {
                            VStack(spacing: 0) {
                                TitleBar(title: "Companies", subtitle: "38 watched · 12 with open jobs that pass")
                                PageHeader(scopes: [("Watching", "38"), ("With open jobs", "12"), ("Suggested", "24")], selectedScope: "Watching", filters: [], search: "Search companies", addTitle: "Add")
                                proposedTable
                            }
                        }
                    }
                }
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("What changes").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Domain, Watched since and Found via take half the row", fix: "They move to optional columns and the company's Overview tab.")
                    ProblemNote(number: 2, problem: "Open jobs aren't on the page at all", fix: "Open jobs that pass the screen, and the best match among them, in the brief's word and tone.")
                    ProblemNote(number: 3, problem: "People and You know are two bare numbers", fix: "One People column: who you know, as faces, and the count.")
                    ProblemNote(number: 4, problem: "Your applications are on another page", fix: "Where you stand: the phase of your application, or Today for a new job.")
                    ProblemNote(number: 5, problem: "A company with no board, or one being researched, looks like any other", fix: "The row says so and offers the step: Find board, or the research's progress.")
                    ProblemNote(number: 6, problem: "Add and From suggestions sit in the window toolbar, over the inspector", fix: "Suggested is a scope; Add is in the page header.")
                }
                .frame(width: 440)
            }
        }
    }

    private var currentTable: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                ForEach(["Company", "Domain", "Watched since", "Job board", "People", "You know", "Found via"], id: \.self) { title in
                    Text(title).frame(width: title == "Job board" ? 260 : title == "People" || title == "You know" ? 80 : 150, alignment: .leading).padding(.leading, 10)
                }
                Spacer(minLength: 0)
            }
            .font(.ui(11, .medium)).foregroundStyle(secondaryInk).frame(height: 28)
            .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
            ForEach(Array(companies.enumerated()), id: \.offset) { index, company in
                HStack(spacing: 0) {
                    HStack(spacing: 6) {
                        if index == 0 { UnseenDot() }
                        Text(company.0)
                    }
                    .frame(width: 150, alignment: .leading).padding(.leading, 10)
                    Text(company.0.lowercased().replacingOccurrences(of: " ", with: "") + ".example").frame(width: 150, alignment: .leading).padding(.leading, 10)
                    Text("12 Sep 2026").frame(width: 150, alignment: .leading).padding(.leading, 10)
                    Text(company.4.isEmpty || company.4 == "No board found" ? "–" : "\(company.4) · boards.\(company.4.lowercased()).io/\(company.0.lowercased().prefix(6))")
                        .lineLimit(1).frame(width: 260, alignment: .leading).padding(.leading, 10)
                    Text("\(index * 3 % 7)").frame(width: 80, alignment: .leading).padding(.leading, 10)
                    Text(company.7 > 0 ? "\(company.7)" : "–").fontWeight(company.7 > 0 ? .semibold : .regular).frame(width: 80, alignment: .leading).padding(.leading, 10)
                    Text(index % 2 == 0 ? "LinkedIn follows" : "startups.gallery").frame(width: 150, alignment: .leading).padding(.leading, 10)
                    Spacer(minLength: 0)
                }
                .font(.ui(12.5)).foregroundStyle(ink).frame(height: 26)
                .background(index % 2 == 1 ? Color.black.opacity(0.025) : .clear)
            }
        }
        .padding(.bottom, Space.s)
    }

    private var proposedTable: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                Text("Company").frame(maxWidth: .infinity, alignment: .leading).padding(.leading, 38)
                Text("Open jobs").frame(width: 190, alignment: .leading)
                Text("People you know").frame(width: 150, alignment: .leading)
                Text("You").frame(width: 130, alignment: .leading)
                Text("Board").frame(width: 150, alignment: .leading)
            }
            .font(.ui(11, .medium)).foregroundStyle(secondaryInk)
            .padding(.horizontal, Space.m).frame(height: 30)
            ForEach(Array(companies.enumerated()), id: \.offset) { index, company in
                HStack(spacing: 0) {
                    Monogram(letters: company.1, hue: Color(hex: company.2), size: 26).padding(.trailing, Space.m)
                    VStack(alignment: .leading, spacing: 1) {
                        HStack(spacing: 6) {
                            Text(company.0).font(.ui(13, .semibold)).foregroundStyle(ink)
                            if index == 0 { UnseenDot() }
                        }
                        HStack(spacing: 5) {
                            if company.9 == "research" { Spinner().scaleEffect(0.8) }
                            Text(company.9 == "research" ? "Researching: reading its careers page, step 3 of 5" : company.3).font(.ui(11.5)).foregroundStyle(secondaryInk)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    HStack(spacing: 6) {
                        if let open = company.5 {
                            Text(open).font(.ui(12)).foregroundStyle(ink)
                            if let match = company.6 { Chip(text: "Best: \(match.0)", tone: match.1) }
                        } else if company.9 != "research" {
                            Text("None that pass").font(.ui(12)).foregroundStyle(tertiaryInk)
                        }
                    }
                    .frame(width: 190, alignment: .leading)
                    HStack(spacing: -6) {
                        ForEach(0..<company.7, id: \.self) { person in
                            Text(["AK", "RC", "JL"][person]).font(.ui(8.5, .bold)).foregroundStyle(secondaryInk)
                                .frame(width: 22, height: 22).background(Color(hex: 0xECECF1), in: Circle())
                                .overlay(Circle().strokeBorder(.white, lineWidth: 1.5))
                        }
                        if company.7 == 0 && company.9 != "research" { Text("—").font(.ui(12)).foregroundStyle(tertiaryInk) }
                    }
                    .frame(width: 150, alignment: .leading)
                    Group {
                        if company.8 == "Today" {
                            Text("New job today").foregroundStyle(Tone.accent.color)
                        } else if company.8 == "—" || company.8.isEmpty {
                            Text(company.8).foregroundStyle(tertiaryInk)
                        } else {
                            HStack(spacing: 4) {
                                Image(systemName: "rectangle.split.3x1").font(.system(size: 10))
                                Text(company.8)
                            }
                            .foregroundStyle(ink)
                        }
                    }
                    .font(.ui(12))
                    .frame(width: 130, alignment: .leading)
                    Group {
                        if company.9 == "Find board" {
                            HStack(spacing: 4) {
                                Image(systemName: "exclamationmark.triangle.fill").font(.system(size: 10)).foregroundStyle(Tone.caution.color)
                                Text("Find board").font(.ui(12, .medium)).foregroundStyle(Tone.accent.color)
                            }
                        } else {
                            Text(company.4).font(.ui(12)).foregroundStyle(secondaryInk)
                        }
                    }
                    .frame(width: 150, alignment: .leading)
                }
                .padding(.horizontal, Space.m)
                .frame(height: 46)
                .background(index == 0 ? Tone.accent.color.opacity(0.10) : .clear, in: RoundedRectangle(cornerRadius: 7))
                .padding(.horizontal, Space.s)
            }
        }
        .padding(.bottom, Space.m)
        .overlay(alignment: .topLeading) { Marker(number: 1).offset(x: -40, y: 50) }
        .overlay(alignment: .topTrailing) { Marker(number: 2).offset(x: -610, y: -26) }
        .overlay(alignment: .topTrailing) { Marker(number: 3).offset(x: -420, y: -26) }
        .overlay(alignment: .topTrailing) { Marker(number: 4).offset(x: -270, y: -26) }
        .overlay(alignment: .topTrailing) { Marker(number: 5).offset(x: -100, y: -26) }
    }
}

// MARK: - Decide

struct DecideWindow: View {
    var body: some View {
        Board(
            eyebrow: "Page · Decide",
            title: "Decide reads the posting, not a summary of it",
            subtitle: "Deciding is the one task that needs the whole job, so Decide stops being a list beside an inspector. The queue is a narrow column, and the job beside it is the job page: the posting and the cards that judge it. P, L and S decide and bring up the next; ↑↓ move without deciding.",
            width: 1640
        ) {
            WindowFrame(width: 1544, height: 1060) {
                HStack(spacing: 0) {
                    Sidebar(selected: "Decide", proposed: true)
                    VStack(spacing: 0) {
                        TitleBar(title: "Decide", subtitle: "7 jobs to decide · 12 decided this week")
                        HStack(alignment: .top, spacing: 0) {
                            queue
                            Rectangle().fill(separator).frame(width: 1)
                            JobPage(showsOutline: false, railWidth: 330, measure: 560).withoutMarkers()
                                .frame(maxWidth: .infinity, alignment: .topLeading)
                        }
                        .overlay(alignment: .top) { Rectangle().fill(separator).frame(height: 1) }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                    .background(surface)
                }
            }
        }
    }

    private var queue: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text("Best match first").font(.ui(11.5, .semibold)).foregroundStyle(secondaryInk)
                Spacer()
                Image(systemName: "arrow.up.arrow.down").font(.system(size: 10)).foregroundStyle(tertiaryInk)
            }
            .padding(.horizontal, Space.s).padding(.vertical, Space.s)
            ForEach(Array(items.enumerated()), id: \.offset) { index, item in
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text(item.0).font(.ui(11, .semibold)).foregroundStyle(item.1.color)
                        Text("·").foregroundStyle(tertiaryInk)
                        Text(item.3).font(.ui(11.5)).foregroundStyle(secondaryInk)
                        Spacer(minLength: 0)
                        if index < 2 { UnseenDot() }
                    }
                    Text(item.2).font(.ui(13, .semibold)).foregroundStyle(ink).lineLimit(1)
                    Text(item.4).font(.ui(11.5)).foregroundStyle(secondaryInk).lineLimit(2).fixedSize(horizontal: false, vertical: true)
                }
                .padding(10)
                .background(index == 0 ? Tone.accent.color.opacity(0.10) : .clear, in: RoundedRectangle(cornerRadius: 8))
                .overlay(alignment: .leading) {
                    if index == 0 { Capsule().fill(Tone.accent.color).frame(width: 3, height: 34).offset(x: 1) }
                }
            }
            Spacer(minLength: 0)
            HStack(spacing: 6) {
                Keycap(key: "P"); Text("Pursue").font(.ui(11)).foregroundStyle(secondaryInk)
                Keycap(key: "L"); Text("Later").font(.ui(11)).foregroundStyle(secondaryInk)
                Keycap(key: "S"); Text("Skip").font(.ui(11)).foregroundStyle(secondaryInk)
            }
            .padding(Space.s)
        }
        .padding(Space.s)
        .frame(width: 300)
        .frame(maxHeight: .infinity, alignment: .top)
    }

    private let items: [(String, Tone, String, String, String)] = [
        ("Strong", .positive, "Senior Product Engineer", "Northwind", "Your checkout cases map onto their rebuild; pay likely clears your target."),
        ("Strong", .positive, "Full-Stack Engineer, Payments", "Initech", "Payments in React and Node, Brazil-based team."),
        ("Possible", .accent, "Staff Frontend Engineer", "Globex", "Strong React fit; the staff scope asks for platform leadership."),
        ("Possible", .accent, "Frontend Engineer II", "Brightline", "A level below yours; pay at the bottom of your range."),
        ("Stretch", .caution, "Senior Software Engineer, Platform", "Umbrella Labs", "Platform and Go heavy; your work is mostly product."),
        ("Stretch", .caution, "Software Engineer, Checkout", "Globex", "Checkout fits, but the take-home is under your minimum."),
        ("Mismatch", .neutral, "Product Engineer", "Acme Robotics", "Hybrid in São Paulo, three days a week."),
    ]
}
