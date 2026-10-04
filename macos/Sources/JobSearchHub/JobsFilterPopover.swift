import JobSearchHubCore
import SwiftUI

/// The Jobs page's filters, each value with how many loaded jobs have it.
struct JobsFilterPopover: View {
    @Binding var filter: JobsFilter
    let choices: JobsFilterChoices
    let newCount: Int

    private let grid = [GridItem(.flexible(), alignment: .leading), GridItem(.flexible(), alignment: .leading)]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Space.l) {
                section("Fit") {
                    ForEach([FitLevel.good, .unclear, .poor], id: \.self) { level in
                        Toggle(describe(level.title, choices.fitLevelCounts[level, default: 0]), isOn: makeShownBinding(level, hiddenIn: \.hiddenFitLevels))
                    }
                }
                section("Hide jobs that fail") {
                    ForEach(JobsColumns.fitChecks) { check in
                        Toggle(
                            describe(check.title, choices.checkFailureCounts[check.name, default: 0]),
                            isOn: makeHiddenBinding(check.name, hiddenIn: \.hiddenCheckFailures)
                        )
                    }
                }
                choiceSection("Source", choices.sources, hiddenIn: \.hiddenSources, getTitle: Job.getSourceName)
                choiceSection("Workplace", choices.workplaces, hiddenIn: \.hiddenWorkplaces)
                choiceSection("Employment", choices.employmentTypes, hiddenIn: \.hiddenEmploymentTypes)
                section("Only") {
                    Toggle(describe("Jobs that list pay", choices.withPayCount), isOn: $filter.showsOnlyJobsWithPay)
                    Toggle(describe("New since your last visit", newCount), isOn: $filter.showsOnlyNewJobs)
                }
                HStack {
                    Spacer()
                    Button("Clear") { filter = JobsFilter() }
                        .disabled(!filter.isActive)
                }
            }
            .toggleStyle(.checkbox)
            .padding(Space.l)
        }
        .frame(width: 420)
        .frame(maxHeight: 600)
    }

    private func section(_ title: String, @ViewBuilder content: () -> some View) -> some View {
        HubSection(title) {
            LazyVGrid(columns: grid, alignment: .leading, spacing: Space.s, content: content)
        }
    }

    /// A section of values the jobs have, plus any hidden now that no loaded job has, so it can be shown again.
    private func choiceSection(
        _ title: String, _ values: [JobsFilterChoice], hiddenIn hidden: WritableKeyPath<JobsFilter, Set<String>>,
        getTitle: @escaping (String) -> String = { $0 }
    ) -> some View {
        let absentHidden = filter[keyPath: hidden].subtracting(values.map(\.value)).sorted()
        let allValues = values + absentHidden.map { JobsFilterChoice(value: $0, title: getTitle($0), count: 0) }
        return section(title) {
            ForEach(allValues) { choice in
                Toggle(describe(choice.title, choice.count), isOn: makeShownBinding(choice.value, hiddenIn: hidden))
            }
        }
    }

    private func describe(_ title: String, _ count: Int) -> String {
        "\(title) (\(count))"
    }

    /// On while the value shows, that is, while it isn't in the hidden set.
    private func makeShownBinding<Value: Hashable>(_ value: Value, hiddenIn hidden: WritableKeyPath<JobsFilter, Set<Value>>) -> Binding<Bool> {
        Binding(
            get: { !filter[keyPath: hidden].contains(value) },
            set: { isShown in
                if isShown { filter[keyPath: hidden].remove(value) } else { filter[keyPath: hidden].insert(value) }
            }
        )
    }

    /// On while the value is in the hidden set.
    private func makeHiddenBinding(_ value: String, hiddenIn hidden: WritableKeyPath<JobsFilter, Set<String>>) -> Binding<Bool> {
        Binding(
            get: { filter[keyPath: hidden].contains(value) },
            set: { isHidden in
                if isHidden { filter[keyPath: hidden].insert(value) } else { filter[keyPath: hidden].remove(value) }
            }
        )
    }
}
