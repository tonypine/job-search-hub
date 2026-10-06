import JobSearchHubCore
import SwiftUI

/// The Jobs header's *+ Filter* menu: each filter with how many loaded jobs
/// it concerns. A filter that is on shows as a chip in the header.
struct JobsFilterMenu: View {
    @Binding var filter: JobsFilter
    let choices: JobsFilterChoices
    let newCount: Int

    var body: some View {
        Button(describe("Passes screen", choices.fitLevelCounts[.good, default: 0])) {
            filter.hiddenFitLevels = JobsFilter.passesScreenHiddenLevels
        }
        .disabled(filter.hiddenFitLevels == JobsFilter.passesScreenHiddenLevels)
        Toggle(describe("Lists pay", choices.withPayCount), isOn: $filter.showsOnlyJobsWithPay)
        Toggle(describe("New since your last visit", newCount), isOn: $filter.showsOnlyNewJobs)
        Divider()
        Menu("Screen") {
            ForEach([FitLevel.good, .unclear, .poor], id: \.self) { level in
                Toggle(describe(level.title, choices.fitLevelCounts[level, default: 0]), isOn: makeShownBinding(level, hiddenIn: \.hiddenFitLevels))
            }
        }
        Menu("Hide jobs that fail") {
            ForEach(JobsColumns.fitChecks) { check in
                Toggle(
                    describe(check.title, choices.checkFailureCounts[check.name, default: 0]),
                    isOn: makeHiddenBinding(check.name, hiddenIn: \.hiddenCheckFailures)
                )
            }
        }
        choiceMenu("Source", choices.sources, hiddenIn: \.hiddenSources, getTitle: Job.getSourceName)
        choiceMenu("Workplace", choices.workplaces, hiddenIn: \.hiddenWorkplaces)
        choiceMenu("Employment", choices.employmentTypes, hiddenIn: \.hiddenEmploymentTypes)
        Divider()
        Button("Clear filters") { filter = JobsFilter() }
            .disabled(!filter.isActive)
    }

    /// A menu of values the jobs have, plus any hidden now that no loaded job has, so it can be shown again.
    private func choiceMenu(
        _ title: String, _ values: [JobsFilterChoice], hiddenIn hidden: WritableKeyPath<JobsFilter, Set<String>>,
        getTitle: @escaping (String) -> String = { $0 }
    ) -> some View {
        let absentHidden = filter[keyPath: hidden].subtracting(values.map(\.value)).sorted()
        let allValues = values + absentHidden.map { JobsFilterChoice(value: $0, title: getTitle($0), count: 0) }
        return Menu(title) {
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
