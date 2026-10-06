import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PeopleModel {
    private(set) var people: [RelatedPerson] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var filter = PeopleFilter()
    var search = ""
    var selectedKey: String?

    /// The people the filter keeps whose name, company or role holds the search.
    var shownPeople: [RelatedPerson] {
        let query = search.trimmingCharacters(in: .whitespaces)
        let filtered = filter.apply(to: people)
        guard !query.isEmpty else { return filtered }
        return filtered.filter { person in
            [person.name, person.companyName, person.role].contains { $0?.localizedStandardContains(query) == true }
        }
    }
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
                    header
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
            }
        }
        .navigationTitle("People")
        .navigationSubtitle(PeopleFilter.describeCounts(model.people))
    }

    /// The page's controls, in its header rather than in the window's
    /// toolbar: the relations as scopes, the filters that are on as chips.
    private var header: some View {
        PageHeader(chips: filterChips) {
            TabStrip(items: relationScopes, selection: $model.filter.relation)
        } trailing: {
            PageSearchField(text: $model.search, prompt: "Search people")
                .help("Search names, companies and roles")
        } filterMenu: {
            Toggle("Hiring now", isOn: $model.filter.hiringNowOnly)
                .help("Only people whose company has open jobs in the feed")
            Toggle("Unanswered", isOn: $model.filter.unansweredOnly)
                .help("Only people who wrote and you never answered")
        }
    }

    /// Everyone, then each relation, with how many people it holds.
    private var relationScopes: [TabStripItem<PersonRelation?>] {
        [TabStripItem<PersonRelation?>(id: nil, title: "All", count: model.people.count)]
            + PersonRelation.allCases.map { relation in
                TabStripItem<PersonRelation?>(id: relation, title: relation.pluralTitle, count: model.people.count { $0.relation == relation })
            }
    }

    private var filterChips: [PageFilterChip] {
        var chips: [PageFilterChip] = []
        if model.filter.hiringNowOnly {
            chips.append(PageFilterChip(id: "hiringNow", title: "Hiring now") { model.filter.hiringNowOnly = false })
        }
        if model.filter.unansweredOnly {
            chips.append(PageFilterChip(id: "unanswered", title: "Unanswered") { model.filter.unansweredOnly = false })
        }
        return chips
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
                ContentUnavailableView {
                    Label("No one matches the filters", systemImage: "line.3.horizontal.decrease.circle")
                } actions: {
                    Button("Clear filters") {
                        model.filter = PeopleFilter()
                        model.search = ""
                    }
                }
            }
        }
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }
}
