import JobSearchHubCore
import SwiftUI

/// The job criteria in Settings: what the feeds search by, and what each
/// job's fit is judged against.
struct JobCriteriaSection: View {
    let client: HubClient
    @State private var editor = JobCriteriaEditor()

    var body: some View {
        @Bindable var editor = editor
        Section {
            listField("Roles", text: $editor.draft.roles, prompt: "Senior Full-Stack Engineer, Product Engineer")
            listField("Rules a title out", text: $editor.draft.excludedRoleTerms, prompt: "Sales, Manager, Analyst, Designer")
            listField("Search terms", text: $editor.draft.searchTerms, prompt: "react, typescript")
            listField("Technologies", text: $editor.draft.technologies, prompt: "TypeScript, React, Node.js")
            listField("Levels", text: $editor.draft.seniorityLevels, prompt: "Senior, Staff")
            TextField("Home country", text: $editor.draft.homeCountry, prompt: Text("Brazil"))
            listField("Hires from", text: $editor.draft.eligibleLocationTerms, prompt: "Brazil, LATAM, Americas, worldwide")
            listField("Rules me out", text: $editor.draft.ineligibleLocationTerms, prompt: "must reside in the US")
            listField("Timezones that work", text: $editor.draft.workableTimezoneTerms, prompt: "US business hours, EST, your local time zone")
            listField("Timezones that don't", text: $editor.draft.unworkableTimezoneTerms, prompt: "APAC, AEST")
            Toggle("Refuse hourly work", isOn: $editor.draft.refuseHourlyWork)
        } header: {
            Text("Job criteria")
        } footer: {
            Text("Separate items with commas. Feeds search by the search terms from the home country; each job's fit is judged against the rest.")
                .foregroundStyle(.secondary)
        }

        Section {
            Toggle("Judge pay by take-home", isOn: $editor.draft.judgesTakeHome)
            if editor.draft.judgesTakeHome {
                TextField("Currency", text: $editor.draft.takeHome.currency, prompt: Text("BRL"))
                TextField("Minimum a month", value: $editor.draft.takeHome.minimumMonthly, format: .number)
                TextField("Target a month", value: $editor.draft.takeHome.targetMonthly, format: .number)
                hiringRow("CLT", hiring: $editor.draft.takeHome.clt)
                hiringRow("PJ", hiring: $editor.draft.takeHome.pj)
                hiringRow("Contractor abroad", hiring: $editor.draft.takeHome.foreignContractor)
            }
            HStack {
                if let errorMessage = editor.errorMessage {
                    Text(errorMessage).foregroundStyle(.red)
                }
                Spacer()
                if editor.isSaving {
                    ProgressView().controlSize(.small)
                }
                Button("Revert") { editor.revert() }
                    .disabled(!editor.hasChanges || editor.isSaving)
                Button("Save criteria") { Task { await editor.save(with: client) } }
                    .disabled(!editor.hasChanges || editor.isSaving)
            }
        } header: {
            Text("Take-home pay")
        } footer: {
            Text("A job's published pay is converted at the day's rate and reduced by the share of each way you could be hired. Pay under the minimum marks a job a poor fit. The shares are estimates; tune them to your own numbers.")
                .foregroundStyle(.secondary)
        }
        .task { await editor.load(with: client) }
    }

    private func listField(_ title: String, text: Binding<String>, prompt: String) -> some View {
        TextField(title, text: text, prompt: Text(prompt), axis: .vertical)
            .lineLimit(1...4)
    }

    private func hiringRow(_ title: String, hiring: Binding<HiringTakeHome>) -> some View {
        LabeledContent(title) {
            HStack {
                TextField("Share", value: hiring.share, format: .percent.precision(.fractionLength(0...1)))
                    .frame(width: 70)
                    .multilineTextAlignment(.trailing)
                Text("of each payment,").foregroundStyle(.secondary)
                TextField("Payments", value: hiring.paymentsPerYear, format: .number.precision(.fractionLength(0...2)))
                    .frame(width: 60)
                    .multilineTextAlignment(.trailing)
                Text("payments a year").foregroundStyle(.secondary)
            }
            .labelsHidden()
        }
    }
}
