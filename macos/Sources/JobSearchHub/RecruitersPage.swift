import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class RecruitersModel {
    private(set) var recruiters: [RecruiterConversation] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var filter = RecruiterFilter()
    var selectedID: UUID?

    var shownRecruiters: [RecruiterConversation] { filter.apply(to: recruiters) }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            recruiters = try await client.get("v1/recruiters", as: RecruitersResponse.self).recruiters
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the recruiters", error)
        }
    }
}

/// The recruiters who wrote to the owner on LinkedIn, the latest first,
/// flagged by what their company has open now and whether the owner answered.
/// The selected one opens in the window's inspector.
struct RecruitersPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @State private var model = RecruitersModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                table
                    .task { await model.load(with: client) }
                    .onChange(of: events.revision) { Task { await model.load(with: client) } }
                    .onChange(of: model.selectedID, initial: true) {
                        details.show(model.selectedID.map(InspectorSubject.person), from: .recruiters)
                    }
                    .onChange(of: details.getEntry(on: .recruiters)) {
                        if details.getEntry(on: .recruiters) == nil { model.selectedID = nil }
                    }
                    .toolbar {
                        Toggle("Hiring now", systemImage: "briefcase", isOn: $model.filter.hiringNowOnly)
                            .help("Only recruiters whose company has open jobs in the feed")
                        Toggle("Unanswered", systemImage: "arrowshape.turn.up.left", isOn: $model.filter.unansweredOnly)
                            .help("Only conversations you never answered")
                    }
            }
        }
        .navigationTitle("Recruiters")
        .navigationSubtitle(describeCounts())
    }

    private var table: some View {
        Table(model.shownRecruiters, selection: $model.selectedID) {
            TableColumn("Recruiter") { recruiter in
                Text(recruiter.startedByName).help(recruiter.starterPosition ?? "")
            }
            TableColumn("Company") { recruiter in
                HStack(spacing: Space.s) {
                    Text(recruiter.hiringCompany ?? "–")
                    if recruiter.isAgency {
                        ToneChip("Agency", tone: .neutral)
                    }
                }
            }
            TableColumn("Role") { recruiter in Text(recruiter.role ?? "").help(recruiter.role ?? "") }
            TableColumn("Openings") { recruiter in
                Text(recruiter.openingsText).fontWeight(recruiter.fittingJobs > 0 ? .semibold : .regular)
                    .foregroundStyle(recruiter.fittingJobs > 0 ? AnyShapeStyle(Tone.positive.color) : AnyShapeStyle(.primary))
            }
            .width(110)
            TableColumn("Answered") { recruiter in Text(recruiter.ownerWrote ? "Yes" : "No").foregroundStyle(recruiter.ownerWrote ? .secondary : .primary) }
                .width(70)
            TableColumn("Last message") { recruiter in
                Text(recruiter.lastMessageAt.map { $0.formatted(date: .abbreviated, time: .omitted) } ?? "–")
            }
            .width(110)
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            } else if model.recruiters.isEmpty && !model.isLoading {
                ContentUnavailableView(
                    "No recruiters yet", systemImage: "person.crop.rectangle.stack",
                    description: Text("Import your LinkedIn archive in Settings › Accounts. The hub reads the conversations others started and lists the recruiters here.")
                )
            }
        }
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }

    private func describeCounts() -> String {
        let hiring = model.recruiters.filter(\.isHiringNow).count
        let unanswered = model.recruiters.filter { !$0.ownerWrote }.count
        return "\(model.recruiters.count) recruiters · \(hiring) hiring now · \(unanswered) unanswered"
    }
}
