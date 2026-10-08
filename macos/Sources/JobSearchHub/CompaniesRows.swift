import JobSearchHubCore
import SwiftUI

/// A row of the Companies table: a company the hub holds, a suggestion to
/// research, or a research whose company the hub doesn't hold yet.
enum CompanyRow: Identifiable {
    case company(CompanySummary)
    case suggestion(CompanySuggestion)
    case research(String)

    var id: String {
        switch self {
        case let .company(summary): Self.getID(of: summary.id)
        case let .suggestion(suggestion): "suggestion:" + suggestion.id
        case let .research(target): "research:" + target
        }
    }

    var name: String {
        switch self {
        case let .company(summary): summary.company.name
        case let .suggestion(suggestion): suggestion.organization
        case let .research(target): target
        }
    }

    var summary: CompanySummary? {
        if case let .company(summary) = self { summary } else { nil }
    }

    static func getID(of companyID: UUID) -> String {
        "company:" + companyID.uuidString
    }

    /// The company a row's ID names; nil for a suggestion or a research.
    static func getCompanyID(from rowID: String) -> UUID? {
        guard rowID.hasPrefix("company:") else { return nil }
        return UUID(uuidString: String(rowID.dropFirst("company:".count)))
    }
}

/// The cells of the Companies table's two-line rows. Each keeps one height
/// whatever the column's width, and every line is truncated: rows whose
/// height follows the table's width can loop the window's layout as the
/// inspector opens (#54, #66).
extension View {
    func companyRowCell(alignment: Alignment = .leading) -> some View {
        frame(maxWidth: .infinity, minHeight: JobRowLayout.cellHeight, maxHeight: JobRowLayout.cellHeight, alignment: alignment)
    }
}

/// The company: its monogram, its name in semibold, and under it where and
/// how big it is, why it's suggested, or the step its research is on.
struct CompanyCell: View {
    let row: CompanyRow
    /// The research's step when this row's company is being researched.
    let researchStep: ResearchStep?

    var body: some View {
        HStack(spacing: Space.m) {
            Monogram(row.name)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 6) {
                    Text(row.name).fontWeight(.semibold).lineLimit(1).help(row.name)
                    if let summary = row.summary, summary.unseenUpdates > 0 {
                        UnseenDot(count: summary.unseenUpdates)
                    }
                }
                if let researchStep {
                    HStack(spacing: Space.xs) {
                        ProgressView().controlSize(.mini)
                        Text(researchStep.line).lineLimit(1).help(researchStep.line)
                    }
                    .font(.hubSecondary)
                    .foregroundStyle(.secondary)
                } else {
                    Text(context).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(1).help(context)
                }
            }
            Spacer(minLength: 0)
        }
        .companyRowCell()
    }

    private var context: String {
        switch row {
        case let .company(summary): summary.contextLine
        case let .suggestion(suggestion): suggestion.origin
        case .research: "Researching"
        }
    }
}

/// The open jobs that pass the screen, "3 open", and the best match among
/// them as a chip, "Best: Strong"; "None that pass" otherwise.
struct OpenJobsCell: View {
    let fittingJobs: Int
    let bestMatch: JobMatch?

    var body: some View {
        HStack(spacing: Space.s) {
            if fittingJobs > 0 {
                Text("\(fittingJobs) open").monospacedDigit()
                if let bestMatch {
                    ToneChip("Best: \(bestMatch.title)", tone: bestMatch.tone)
                        .help("The best match the briefs found among them")
                }
            } else {
                Text("None that pass").foregroundStyle(.tertiary)
            }
        }
        .font(.hubSecondary)
        .lineLimit(1)
        .help(fittingJobs == 1 ? "1 open job passes the screen" : "\(fittingJobs) open jobs pass the screen")
        .companyRowCell()
    }
}

/// Up to three faces of the people you know there, and how many.
struct KnownPeopleCell: View {
    let names: [String]
    let count: Int

    var body: some View {
        Group {
            if count > 0 {
                FacePile(names: names, count: count)
            } else {
                Text("–").foregroundStyle(.tertiary)
            }
        }
        .companyRowCell()
    }
}

/// Where you stand: the open application's phase, or *New job today*.
struct StandingCell: View {
    let standing: CompanyStanding?

    var body: some View {
        Group {
            switch standing {
            case let .phase(name):
                Label(name, systemImage: "rectangle.split.3x1")
                    .foregroundStyle(.primary)
                    .help("Your application is in \(name)")
            case .newJobToday:
                Text(CompanyStanding.newJobToday.text)
                    .foregroundStyle(Tone.accent.color)
                    .help("A job that passes the screen appeared today")
            case nil:
                Text("–").foregroundStyle(.tertiary)
            }
        }
        .font(.hubSecondary)
        .lineLimit(1)
        .companyRowCell()
    }
}

/// The board the hub reads the company's jobs from, or *Find board* with a
/// caution symbol when it found none, which runs the job finder as the
/// company's inspector does.
struct BoardCell: View {
    let summary: CompanySummary
    let isFinding: Bool
    let onFind: () -> Void

    var body: some View {
        Group {
            if let boardName = summary.boardName {
                Text(boardName)
                    .foregroundStyle(.secondary)
                    .help(summary.jobBoards.first?.summaryLine ?? boardName)
            } else if isFinding {
                HStack(spacing: Space.xs) {
                    ProgressView().controlSize(.mini)
                    Text("Finding board…").foregroundStyle(.secondary)
                }
            } else {
                Button(action: onFind) {
                    HStack(spacing: Space.xs) {
                        Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Tone.caution.color)
                        Text("Find board").foregroundStyle(Tone.accent.color)
                    }
                }
                .buttonStyle(.plain)
                .help("No board found. An agent looks for the company's open roles: it sets the job board when the hub reads it, or records the roles off the careers page")
                .accessibilityLabel("No board found. Find board")
            }
        }
        .font(.hubSecondary)
        .lineLimit(1)
        .companyRowCell()
    }
}
