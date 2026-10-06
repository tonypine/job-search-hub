import JobSearchHubCore
import SwiftUI

/// The job criteria on the Criteria page. Search groups them by the
/// question they answer: what the owner looks for, where they can work and
/// what rules a job out, each list a token field. Pay holds the take-home
/// the job must reach. ContentView holds the editor, so unsaved edits
/// outlive a switch to another scope, and leaving the page asks first.
struct JobCriteriaForm: View {
    let scope: CriteriaScope
    let editor: JobCriteriaEditor

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Space.xl) {
                if scope == .pay {
                    pay
                } else {
                    search
                }
            }
            .padding(Space.xl)
            .frame(maxWidth: 820, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder
    private var search: some View {
        @Bindable var editor = editor
        CriteriaGroup("What you look for", help: "Feeds search by the search terms, and the screen checks every job's title, level and stack against the rest.") {
            row("Roles") { tokens("Roles", .roles) }
            row("Levels") { tokens("Levels", .seniorityLevels) }
            row("Technologies") { tokens("Technologies", .technologies) }
            row("Search terms") { tokens("Search terms", .searchTerms) }
        }
        CriteriaGroup("Where you can work", help: whereHelp) {
            row("Home country") {
                Picker("Home country", selection: $editor.draft.homeCountry) {
                    Text("None").tag("")
                    ForEach(HomeCountries.makeChoices(keeping: editor.draft.homeCountry), id: \.self) { name in
                        Text(name).tag(name)
                    }
                }
                .labelsHidden()
                .fixedSize()
            }
            row("Hires from") { tokens("Hires from", .eligibleLocationTerms) }
            row("Rules me out") { tokens("Rules me out", .ineligibleLocationTerms) }
            row("Timezones that work") { tokens("Timezones that work", .workableTimezoneTerms) }
            row("Timezones that don't") { tokens("Timezones that don't", .unworkableTimezoneTerms) }
        }
        CriteriaGroup("What rules a job out", help: "A title with one of these words, or hourly pay when you refuse it, fails the job's screen.") {
            row("Title words") { tokens("Title words", .excludedRoleTerms) }
            row("Hourly work") {
                Toggle("Refuse it", isOn: $editor.draft.refuseHourlyWork)
                    .toggleStyle(.switch)
                    .accessibilityLabel("Refuse hourly work")
            }
        }
    }

    private var whereHelp: String {
        let rule = "A job passes where it hires from one of these, and fails where it rules you out."
        let country = editor.draft.homeCountry.trimmingCharacters(in: .whitespaces)
        return country.isEmpty ? rule : "From \(country). " + rule
    }

    @ViewBuilder
    private var pay: some View {
        @Bindable var editor = editor
        CriteriaGroup(
            "Take-home pay",
            help: "A job's published pay is converted at the day's rate and reduced by how you'd be hired. Under the minimum, it fails the screen."
        ) {
            row("Judge pay") {
                Toggle("By take-home", isOn: $editor.draft.judgesTakeHome)
                    .toggleStyle(.switch)
                    .accessibilityLabel("Judge pay by take-home")
            }
            if editor.draft.judgesTakeHome {
                row("Currency") {
                    TextField("Currency", text: $editor.draft.takeHome.currency, prompt: Text("BRL"))
                        .labelsHidden()
                        .frame(width: 90)
                }
                row("Minimum a month") { amountField("Minimum a month", value: $editor.draft.takeHome.minimumMonthly) }
                row("Target a month") { amountField("Target a month", value: $editor.draft.takeHome.targetMonthly) }
            }
        }
        if editor.draft.judgesTakeHome {
            CriteriaGroup("Ways to be hired", help: "The share of each payment you keep, and how many payments a year. They're estimates; tune them to your numbers.") {
                Grid(alignment: .leading, horizontalSpacing: Space.l, verticalSpacing: Space.m) {
                    GridRow {
                        Text("")
                        Text("Share you keep").font(.hubCaption).foregroundStyle(.secondary)
                        Text("Payments a year").font(.hubCaption).foregroundStyle(.secondary)
                    }
                    hiringRow("CLT", hiring: $editor.draft.takeHome.clt)
                    hiringRow("PJ", hiring: $editor.draft.takeHome.pj)
                    hiringRow("Contractor abroad", hiring: $editor.draft.takeHome.foreignContractor)
                }
            }
        }
    }

    private func tokens(_ title: String, _ list: JobCriteriaList) -> some View {
        let editor = editor
        return TokenField(
            title: title, tokens: Binding(get: { editor.draft[list] }, set: { editor.draft[list] = $0 }),
            text: Binding(get: { editor.draft.unaddedText[list] ?? "" }, set: { editor.draft.unaddedText[list] = $0.isEmpty ? nil : $0 }),
            tone: list.tone
        )
    }

    private func row<Field: View>(_ label: String, @ViewBuilder field: () -> Field) -> some View {
        HStack(alignment: .top, spacing: Space.l) {
            Text(label)
                .frame(width: 150, alignment: .leading)
                .padding(.top, 5)
            field()
                .frame(minHeight: 28)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func amountField(_ title: String, value: Binding<Double>) -> some View {
        TextField(title, value: value, format: .number)
            .labelsHidden()
            .multilineTextAlignment(.trailing)
            .frame(width: 120)
    }

    private func hiringRow(_ title: String, hiring: Binding<HiringTakeHome>) -> some View {
        GridRow {
            Text(title).frame(width: 150, alignment: .leading)
            TextField("\(title) share", value: hiring.share, format: .percent.precision(.fractionLength(0...1)))
                .labelsHidden()
                .multilineTextAlignment(.trailing)
                .frame(width: 80)
            TextField("\(title) payments a year", value: hiring.paymentsPerYear, format: .number.precision(.fractionLength(0...2)))
                .labelsHidden()
                .multilineTextAlignment(.trailing)
                .frame(width: 80)
        }
    }
}

/// One question on the Criteria page: its title, one line on what its
/// fields do, and the fields in a card.
private struct CriteriaGroup<Content: View>: View {
    let title: String
    let help: String
    @ViewBuilder let content: () -> Content

    init(_ title: String, help: String, @ViewBuilder content: @escaping () -> Content) {
        self.title = title
        self.help = help
        self.content = content
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.hubSection)
                Text(help).font(.hubSecondary).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isHeader)
            VStack(alignment: .leading, spacing: Space.m) { content() }
                .hubCard()
        }
    }
}
