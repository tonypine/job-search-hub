// Forms: the Criteria page and the Settings window, before and after.
import SwiftUI

// MARK: - Grouped form pieces, as macOS draws them

struct FormGroup<Content: View>: View {
    var title: String?
    var footer: String?
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let title { Text(title).font(.ui(12, .semibold)).foregroundStyle(ink).padding(.leading, 4) }
            VStack(spacing: 0) { content }
                .background(surface, in: RoundedRectangle(cornerRadius: 9))
                .overlay(RoundedRectangle(cornerRadius: 9).strokeBorder(separator))
            if let footer {
                Text(footer).font(.ui(11)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true).padding(.horizontal, 4)
            }
        }
    }
}

struct FormRow<Trailing: View>: View {
    let label: String
    var isLast = false
    @ViewBuilder let trailing: Trailing
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Text(label).font(.ui(12.5)).foregroundStyle(ink)
            Spacer(minLength: Space.s)
            trailing
        }
        .padding(.horizontal, Space.m).padding(.vertical, 8)
        .overlay(alignment: .bottom) { if !isLast { Rectangle().fill(separator).frame(height: 1).padding(.leading, Space.m) } }
    }
}

struct TextFieldBox: View {
    let text: String
    var width: CGFloat = 260
    var isPrompt = false
    var body: some View {
        Text(text).font(.ui(12)).foregroundStyle(isPrompt ? tertiaryInk : ink).lineLimit(2).multilineTextAlignment(.trailing)
            .frame(width: width, alignment: .trailing)
    }
}

// MARK: - Criteria

struct CriteriaBeforeAfter: View {
    var body: some View {
        Board(
            eyebrow: "Page section · Criteria and Settings forms",
            title: "Criteria reads like what it is: who you are as a candidate",
            subtitle: "Criteria decide what every page shows, but the page is a settings form: eleven comma-separated text fields under one heading, the pay rules in a second group, and Save at the end of it. The proposal groups the fields by the question they answer, turns lists into tokens, and keeps Save in reach.",
            width: 1900
        ) {
            HStack(alignment: .top, spacing: 48) {
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Today", subtitle: "The Criteria page.", tone: .negative)
                    WindowFrame(width: 600, height: 1010) {
                        VStack(alignment: .leading, spacing: 0) {
                            TitleBar(title: "Criteria", subtitle: "What the hub searches for, and screens every job against")
                            currentForm.padding(.horizontal, Space.xl).padding(.top, Space.s)
                        }
                    }
                }
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Proposed", subtitle: "The same fields, three questions, tokens, and a save bar.", tone: .positive)
                    WindowFrame(width: 760, height: 1010) {
                        VStack(alignment: .leading, spacing: 0) {
                            TitleBar(title: "Criteria", subtitle: "Screens 48 open jobs · 31 pass")
                            PageHeader(scopes: [("Search", nil), ("Pay", nil), ("Pipeline phases", nil)], selectedScope: "Search", addFilter: false, search: "Find a setting", addTitle: nil)
                                .marker(3, .topTrailing, x: -420, y: 2)
                            proposedForm.padding(Space.xl)
                            Spacer(minLength: 0)
                        }
                        .overlay(alignment: .bottom) { saveBar.padding(Space.l) }
                    }
                }
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("What changes").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Lists are comma-separated text", fix: "Token fields: each value a token you can remove, a field to type the next. Values that rule a job out are red, values that let one in green.")
                    ProblemNote(number: 2, problem: "Eleven fields under one heading, Job criteria", fix: "Three questions: what you look for, where you can work, what rules a job out. Each with one line on what it does.")
                    ProblemNote(number: 3, problem: "Pay, phases and search share one long scroll", fix: "Search, Pay and Pipeline phases are the page's scopes.")
                    ProblemNote(number: 4, problem: "Save and Revert sit at the end of the second group, out of sight", fix: "A save bar rises from the bottom once something changed, and says how many changes.")
                    ProblemNote(number: 5, problem: "The footer explains commas and feeds", fix: "Help moves into each group's one line; the comma rule goes away with the tokens.")
                    Divider().padding(.vertical, Space.s)
                    Text("The same rules for Settings").font(.ui(15, .semibold)).foregroundStyle(ink)
                    SettingsRowsSample()
                }
                .frame(width: 390)
            }
        }
    }

    private var currentForm: some View {
        VStack(alignment: .leading, spacing: Space.l) {
            FormGroup(title: "Job criteria", footer: "Separate items with commas. Feeds search by the search terms from the home country; each job is screened against the rest.") {
                FormRow(label: "Roles") { TextFieldBox(text: "Senior Full-Stack Engineer, Product Engineer, Senior Frontend Engineer") }
                FormRow(label: "Rules a title out") { TextFieldBox(text: "Sales, Manager, Analyst, Designer, Data") }
                FormRow(label: "Search terms") { TextFieldBox(text: "react, typescript, node") }
                FormRow(label: "Technologies") { TextFieldBox(text: "TypeScript, React, Node.js, PostgreSQL") }
                FormRow(label: "Levels") { TextFieldBox(text: "Senior, Staff") }
                FormRow(label: "Home country") { TextFieldBox(text: "Brazil") }
                FormRow(label: "Hires from") { TextFieldBox(text: "Brazil, LATAM, Latin America, Americas, worldwide") }
                FormRow(label: "Rules me out") { TextFieldBox(text: "must reside in the US, US only, EU only") }
                FormRow(label: "Timezones that work") { TextFieldBox(text: "US business hours, EST, your local time zone") }
                FormRow(label: "Timezones that don't") { TextFieldBox(text: "APAC, AEST") }
                FormRow(label: "Refuse hourly work", isLast: true) { Switch(isOn: true) }
            }
            .marker(1, .topLeading, x: -20, y: 40)
            .marker(2, .topLeading, x: -20, y: -2)
            .marker(5, .bottomLeading, x: -20, y: -10)
            FormGroup(title: "Take-home pay", footer: "A job's published pay is converted at the day's rate and reduced by the share of each way you could be hired. Pay under the minimum fails a job's screen.") {
                FormRow(label: "Judge pay by take-home") { Switch(isOn: true) }
                FormRow(label: "Currency") { TextFieldBox(text: "BRL", width: 80) }
                FormRow(label: "Minimum a month") { TextFieldBox(text: "22,000", width: 80) }
                FormRow(label: "Target a month") { TextFieldBox(text: "30,000", width: 80) }
                FormRow(label: "CLT") { Text("72%  of each payment,  13.3  payments a year").font(.ui(12)).foregroundStyle(secondaryInk) }
                FormRow(label: "PJ") { Text("85%  of each payment,  12  payments a year").font(.ui(12)).foregroundStyle(secondaryInk) }
                FormRow(label: "Contractor abroad", isLast: true) { Text("88%  of each payment,  12  payments a year").font(.ui(12)).foregroundStyle(secondaryInk) }
            }
            .marker(3, .topLeading, x: -20, y: -2)
            HStack {
                Spacer()
                SecondaryButton(title: "Revert")
                SecondaryButton(title: "Save criteria")
            }
            .marker(4, .trailing, x: 26)
        }
    }

    private var proposedForm: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            group("What you look for", "Feeds search by these, and the screen checks every job's title, level and stack against them.") {
                tokenRow("Roles", ["Senior Full-Stack Engineer", "Product Engineer", "Senior Frontend Engineer"])
                tokenRow("Levels", ["Senior", "Staff"])
                tokenRow("Technologies", ["TypeScript", "React", "Node.js", "PostgreSQL"], tone: .positive)
                tokenRow("Search terms", ["react", "typescript", "node"])
            }
            .marker(2, .topLeading, x: -22, y: -2)
            group("Where you can work", "From Brazil. A job passes where it hires from one of these, and fails where it rules you out.") {
                HStack(alignment: .firstTextBaseline) {
                    Text("Home country").font(.ui(12)).foregroundStyle(secondaryInk).frame(width: 130, alignment: .leading)
                    HStack(spacing: 4) {
                        Text("Brazil").font(.ui(12))
                        Image(systemName: "chevron.up.chevron.down").font(.system(size: 8, weight: .semibold)).foregroundStyle(secondaryInk)
                    }
                    .padding(.horizontal, 8).padding(.vertical, 3)
                    .background(surface, in: RoundedRectangle(cornerRadius: Radius.control))
                    .overlay(RoundedRectangle(cornerRadius: Radius.control).strokeBorder(separator))
                    Spacer()
                }
                tokenRow("Hires from", ["Brazil", "LATAM", "Latin America", "Americas", "worldwide"], tone: .positive)
                    .marker(1, .topTrailing, x: 26)
                tokenRow("Rules me out", ["must reside in the US", "US only", "EU only"], tone: .negative)
                tokenRow("Timezones that work", ["US business hours", "EST"], tone: .positive)
                tokenRow("Timezones that don't", ["APAC", "AEST"], tone: .negative)
            }
            group("What rules a job out", "Titles with these words never reach Decide.") {
                tokenRow("Title words", ["Sales", "Manager", "Analyst", "Designer", "Data"], tone: .negative)
                HStack {
                    Text("Hourly work").font(.ui(12)).foregroundStyle(secondaryInk).frame(width: 130, alignment: .leading)
                    Switch(isOn: true)
                    Text("Refuse it").font(.ui(12)).foregroundStyle(ink)
                    Spacer()
                }
            }
        }
    }

    private func group<Content: View>(_ title: String, _ help: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.ui(14, .semibold)).foregroundStyle(ink)
                Text(help).font(.ui(12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
            VStack(alignment: .leading, spacing: Space.m) { content() }
                .padding(Space.l)
                .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
        }
    }

    private func tokenRow(_ label: String, _ tokens: [String], tone: Tone = .neutral) -> some View {
        HStack(alignment: .top) {
            Text(label).font(.ui(12)).foregroundStyle(secondaryInk).frame(width: 130, alignment: .leading).padding(.top, 6)
            TokenField(tokens: tokens, tone: tone)
        }
    }

    private var saveBar: some View {
        HStack(spacing: Space.m) {
            Circle().fill(Tone.accent.color).frame(width: 7, height: 7)
            Text("3 unsaved changes").font(.ui(12.5, .medium)).foregroundStyle(ink)
            Spacer()
            SecondaryButton(title: "Revert")
            PrimaryButton(title: "Save", shortcut: "⌘S")
        }
        .padding(.horizontal, Space.l).padding(.vertical, 10)
        .frame(width: 520)
        .background(surface, in: Capsule())
        .overlay(Capsule().strokeBorder(separator))
        .shadow(color: .black.opacity(0.12), radius: 12, y: 4)
        .marker(4, .leading, x: -30)
    }
}

/// Settings rows as proposed: a tile, the account or service, its state in
/// a word and a dot, and one action; the help behind a ⓘ.
struct SettingsRowsSample: View {
    var body: some View {
        VStack(spacing: 0) {
            row("envelope.fill", Color(hex: 0xEA4335), "Google", "Reads Gmail and Calendar", .positive, "Connected", "Check")
            row("person.2.fill", Color(hex: 0x0A66C2), "LinkedIn import", "1,240 connections · 3 Sep", .neutral, "Imported", "Import…")
            row("iphone", Color(hex: 0x6E6E73), "Phones", "Pixel 9 · paired 2 Sep", .positive, "1 paired", "Pair…")
            row("cpu", Tone.accent.color, "Local models", "qwen3-8b · reading 3 postings", .accent, "Working", "Pause", isLast: true)
        }
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }

    private func row(_ symbol: String, _ color: Color, _ title: String, _ detail: String, _ tone: Tone, _ state: String, _ action: String, isLast: Bool = false) -> some View {
        HStack(spacing: Space.s + 2) {
            Image(systemName: symbol).font(.system(size: 11)).foregroundStyle(.white)
                .frame(width: 24, height: 24).background(color, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 5) {
                    Text(title).font(.ui(12.5, .semibold)).foregroundStyle(ink)
                    Image(systemName: "info.circle").font(.system(size: 10)).foregroundStyle(tertiaryInk)
                }
                HStack(spacing: 4) {
                    Circle().fill(tone.color).frame(width: 6, height: 6)
                    Text("\(state) · \(detail)").font(.ui(11)).foregroundStyle(secondaryInk).lineLimit(1)
                }
            }
            Spacer(minLength: 4)
            SecondaryButton(title: action)
        }
        .padding(.horizontal, Space.m).padding(.vertical, 9)
        .overlay(alignment: .bottom) { if !isLast { Rectangle().fill(separator).frame(height: 1).padding(.leading, 46) } }
    }
}
