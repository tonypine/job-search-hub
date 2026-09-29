import JobSearchHubCore
import SwiftUI

/// The Jobs table's optional columns: board facts, each fit check, and the
/// facts read from postings.
enum JobsColumns {
    struct FitCheckColumn: Identifiable, Sendable {
        /// The check's name in the fit.
        let name: String
        let title: String

        var id: String { "fit." + name }
        var sortComparator: JobsSortComparator { JobsSortComparator(.fitCheck(name)) }
    }

    /// The fit checks, titled as columns; Pay's reason is the take-home.
    static let fitChecks = [
        FitCheckColumn(name: "Role", title: "Role fit"),
        FitCheckColumn(name: "Where they hire", title: "Where they hire"),
        FitCheckColumn(name: "Stack", title: "Stack fit"),
        FitCheckColumn(name: "Level", title: "Level fit"),
        FitCheckColumn(name: "Timezone", title: "Timezone fit"),
        FitCheckColumn(name: "Pay", title: "Take-home pay"),
    ]

    struct BoardFactColumn: Identifiable, Sendable {
        let id: String
        let title: String
        let sortComparator: JobsSortComparator
        let getText: @Sendable (JobListItem) -> String?
    }

    /// The facts the job's board or feed gave, and where it came from.
    static let boardFacts = [
        BoardFactColumn(id: "pay", title: "Pay", sortComparator: JobsSortComparator(.pay)) { $0.job.pay?.summaryLine },
        BoardFactColumn(id: "workplace", title: "Workplace", sortComparator: JobsSortComparator(.workplace)) { $0.job.workplaceType },
        BoardFactColumn(id: "employment", title: "Employment", sortComparator: JobsSortComparator(.employment)) { $0.job.employmentType },
        BoardFactColumn(id: "department", title: "Department", sortComparator: JobsSortComparator(.department)) { $0.job.department },
        BoardFactColumn(id: "published", title: "Published", sortComparator: JobsSortComparator(.published)) {
            $0.job.publishedAt?.formatted(date: .abbreviated, time: .omitted)
        },
        BoardFactColumn(id: "source", title: "Source", sortComparator: JobsSortComparator(.source)) { $0.job.sourceName },
    ]

    static func getFactColumnID(_ key: String) -> String {
        "fact." + key
    }
}

/// A fit check's verdict and reason in a table cell.
struct FitCheckCell: View {
    let check: FitCheck?

    var body: some View {
        if let check {
            Label {
                Text(check.reason).help(check.reason)
            } icon: {
                Image(systemName: symbolName(check.verdict)).foregroundStyle(color(check.verdict))
            }
        } else {
            Text("–")
        }
    }

    private func symbolName(_ verdict: FitVerdict) -> String {
        switch verdict {
        case .yes: "checkmark.circle.fill"
        case .no: "xmark.circle.fill"
        case .unclear: "questionmark.circle"
        }
    }

    private func color(_ verdict: FitVerdict) -> Color {
        switch verdict {
        case .yes: .green
        case .no: .red
        case .unclear: .orange
        }
    }
}

/// The toolbar menu that shows and hides the table's optional columns; the
/// header's own menu does the same.
struct ColumnsMenu: View {
    @Binding var customization: TableColumnCustomization<JobListItem>
    let factColumns: [JobFactColumn]

    var body: some View {
        Menu("Columns", systemImage: "tablecells") {
            Section("From the board") {
                ForEach(JobsColumns.boardFacts) { column in toggle(column.title, id: column.id) }
            }
            Section("Fit") {
                ForEach(JobsColumns.fitChecks) { check in toggle(check.title, id: check.id) }
            }
            if !factColumns.isEmpty {
                Section("Read from the posting") {
                    ForEach(factColumns) { column in toggle(column.title, id: JobsColumns.getFactColumnID(column.key)) }
                }
            }
        }
        .help("Choose the columns the table shows")
    }

    /// A toggle for a column hidden by default: on once the owner shows it.
    private func toggle(_ title: String, id: String) -> some View {
        Toggle(title, isOn: Binding(
            get: { customization[visibility: id] == .visible },
            set: { customization[visibility: id] = $0 ? .visible : .hidden }
        ))
    }
}
