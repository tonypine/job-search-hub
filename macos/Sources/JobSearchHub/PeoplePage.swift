import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PeopleModel {
    private(set) var people: [RelatedPerson] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var filter = PeopleFilter()
    var selectedKey: String?

    var shownPeople: [RelatedPerson] { filter.apply(to: people) }
    var selected: RelatedPerson? { people.first { $0.key == selectedKey } }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            people = try await client.get("v1/people", as: PeopleResponse.self).people
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the people", error)
        }
    }
}

/// Everyone who can get the owner in: contacts from company research,
/// LinkedIn connections and recruiters, and introducers, the latest contacted
/// first. The selected one opens in the window's inspector.
struct PeoplePage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @State private var model = PeopleModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(alignment: .leading, spacing: 0) {
                    table
                    Text("Contacts come from company research, connections and recruiters from your LinkedIn import, introducers from what you add on a company.")
                        .font(.hubCaption).foregroundStyle(.secondary)
                        .padding(.horizontal, Space.l).padding(.vertical, Space.s)
                }
                .task { await model.load(with: client) }
                .onChange(of: events.revision) { Task { await model.load(with: client) } }
                .onChange(of: model.selectedKey, initial: true) {
                    details.show(model.selected.map { InspectorSubject.person($0.reference) }, from: .people)
                }
                .onChange(of: details.getEntry(on: .people)) {
                    if details.getEntry(on: .people) == nil { model.selectedKey = nil }
                }
                .toolbar {
                    Picker("Relation", selection: $model.filter.relation) {
                        Text("All").tag(PersonRelation?.none)
                        ForEach(PersonRelation.allCases) { relation in
                            Text(relation.pluralTitle).tag(PersonRelation?.some(relation))
                        }
                    }
                    .pickerStyle(.segmented)
                    .help("Show one relation")
                    Toggle("Hiring now", systemImage: "briefcase", isOn: $model.filter.hiringNowOnly)
                        .help("Only people whose company has open jobs in the feed")
                    Toggle("Unanswered", systemImage: "arrowshape.turn.up.left", isOn: $model.filter.unansweredOnly)
                        .help("Only people who wrote and you never answered")
                }
            }
        }
        .navigationTitle("People")
        .navigationSubtitle(PeopleFilter.describeCounts(model.people))
    }

    private var table: some View {
        Table(model.shownPeople, selection: $model.selectedKey) {
            TableColumn("Name") { person in
                Text(person.name).fontWeight(.semibold).help(person.whatTheyCanDo)
            }
            TableColumn("Relation") { person in ToneChip(person.relationTitle, tone: .neutral) }
                .width(min: 90, ideal: 120)
            TableColumn("Company") { person in
                HStack(spacing: Space.s) {
                    Text(person.companyName ?? "–")
                    if person.isHiringNow {
                        Image(systemName: "briefcase.fill").foregroundStyle(Tone.positive.color)
                            .help(person.openingsText)
                            .accessibilityLabel("Hiring now: \(person.openingsText)")
                    }
                }
            }
            TableColumn("Role") { person in Text(person.role ?? "").help(person.role ?? "") }
            TableColumn("Last contact") { person in
                Text(person.lastContactAt.map { $0.formatted(.relative(presentation: .named)) } ?? "–")
                    .help(person.lastContactAt.map { $0.formatted(date: .abbreviated, time: .shortened) } ?? "")
            }
            .width(min: 90, ideal: 110)
            TableColumn("Answered") { person in
                switch person.answered {
                case false?: ToneChip("Unanswered", tone: .caution)
                case true?: ToneChip("Answered", tone: .neutral)
                case nil: EmptyView()
                }
            }
            .width(min: 90, ideal: 110)
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            } else if model.people.isEmpty && !model.isLoading {
                ContentUnavailableView(
                    "No people yet", systemImage: Page.people.symbolName,
                    description: Text("Import your LinkedIn archive in Settings › Accounts, research a company, or add someone who can introduce you on a company.")
                )
            } else if model.shownPeople.isEmpty && !model.isLoading {
                ContentUnavailableView("No one matches the filters", systemImage: "line.3.horizontal.decrease.circle")
            }
        }
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }
}
