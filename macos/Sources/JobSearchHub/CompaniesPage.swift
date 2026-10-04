import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompaniesModel {
    private(set) var summaries: [CompanySummary] = []
    private(set) var dossier: CompanyDossier?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var selectedID: UUID?

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            summaries = try await client.get("v1/companies", as: CompaniesResponse.self).companies
            loadError = nil
            if selectedID == nil || !summaries.contains(where: { $0.id == selectedID }) {
                selectedID = summaries.first?.id
            }
        } catch {
            loadError = HubFailure("Couldn't load the companies", error)
        }
    }

    func loadDossier(with client: HubClient) async {
        guard let selectedID else {
            dossier = nil
            return
        }
        do {
            dossier = try await client.get("v1/companies/\(selectedID.uuidString)", as: CompanyDossier.self)
        } catch {
            loadError = HubFailure("Couldn't load the company", error)
        }
    }
}

struct CompaniesPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(CompanyResearch.self) private var research
    @Environment(CompanyJobFinder.self) private var jobFinder
    @State private var isAddingCompany = false
    @State private var isShowingSuggestions = false
    @State private var model = CompaniesModel()
    @State private var side: PanelSide = .details
    private let opensSession: Bool

    init(initialCompanyID: UUID? = nil, opensSession: Bool = false) {
        let model = CompaniesModel()
        model.selectedID = initialCompanyID
        _model = State(initialValue: model)
        _side = State(initialValue: opensSession ? .session : .details)
        self.opensSession = opensSession
    }

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                HStack(spacing: 0) {
                    companyTable
                    Divider()
                    companyPanel(client: client).frame(width: 460)
                }
                .task { await model.load(with: client) }
                .onChange(of: [events.revision, unseen.revision, research.revision, jobFinder.revision]) { Task { await model.load(with: client) } }
                .sheet(isPresented: $isShowingSuggestions) {
                    CompanySuggestionsSheet(client: client) { suggestion in
                        research.start(company: suggestion.researchTarget, foundVia: suggestion.origin, client: client)
                        isShowingSuggestions = false
                        isAddingCompany = true
                    }
                }
                .sheet(isPresented: $isAddingCompany) {
                    AddCompanySheet(client: client, getCompanyName: { id in model.summaries.first { $0.id == id }?.company.name }) { companyID in
                        model.selectedID = companyID
                        Task { await model.load(with: client) }
                    }
                }
                .task(id: model.selectedID) {
                    await model.loadDossier(with: client)
                    if let companyID = model.selectedID {
                        await unseen.markSeen(UpdateSelection(companyID: companyID), with: client)
                    }
                }
                .toolbar {
                    if research.isRunning {
                        Button {
                            isAddingCompany = true
                        } label: {
                            HStack(spacing: Space.s) {
                                ProgressView().controlSize(.small)
                                Text("Researching \(research.company)")
                            }
                        }
                        .help("Show the research's progress")
                    }
                    Button("Suggestions", systemImage: "sparkles") { isShowingSuggestions = true }
                        .help("Companies you follow on LinkedIn, or remote ones on startups.gallery, to research")
                    Button("Add company", systemImage: "plus") { isAddingCompany = true }
                    Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                        .disabled(model.isLoading)
                }
            } else {
                NotConnectedView()
            }
        }
        .navigationTitle("Companies")
    }

    private func companyPanel(client: HubClient) -> some View {
        VStack(spacing: 0) {
            Picker("Show", selection: $side) {
                ForEach(PanelSide.allCases) { side in Text(side.rawValue).tag(side) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(Space.s)
            .disabled(model.selectedID == nil)
            if side == .session, let companyID = model.selectedID {
                ClaudeSessionPane(subject: .company(companyID), client: client, startsOnAppear: opensSession)
            } else {
                DossierPane(dossier: model.dossier, client: client) { Task { await model.loadDossier(with: client) } }
            }
        }
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }

    private var companyTable: some View {
        Table(model.summaries, selection: $model.selectedID) {
            TableColumn("Company") { summary in
                HStack(spacing: Space.s) {
                    UnseenDot(count: summary.unseenUpdates)
                    Text(summary.company.name)
                }
            }
            TableColumn("Domain", value: \.company.domain)
            TableColumn("Watched since") { summary in
                Text(summary.watchedSince.map { $0.formatted(date: .abbreviated, time: .omitted) } ?? "–")
            }
            TableColumn("Job board") { summary in
                Text(summary.jobBoards.first?.summaryLine ?? "–")
            }
            TableColumn("People") { summary in
                Text("\(summary.peopleCount)")
            }
            .width(60)
            TableColumn("You know") { summary in
                Text(summary.connectionCount > 0 ? "\(summary.connectionCount)" : "–")
                    .fontWeight(summary.connectionCount > 0 ? .semibold : .regular)
            }
            .width(70)
            TableColumn("Found via") { summary in
                Text(summary.company.foundVia ?? "")
                    .help(summary.company.foundVia ?? "")
            }
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            }
        }
    }
}

struct DossierPane: View {
    let dossier: CompanyDossier?
    let client: HubClient
    /// Reads the dossier again after a change made from it.
    let onChanged: () -> Void
    @State private var isAddingWarmPath = false
    @State private var failure: HubFailure?
    @State private var isRecordingOutreach = false
    @State private var outreachNote = ""
    @State private var outreachFailure: HubFailure?
    @Environment(CompanyJobFinder.self) private var jobFinder

    var body: some View {
        content
            .onChange(of: jobFinder.revision) { onChanged() }
    }

    @ViewBuilder
    private var content: some View {
        if let dossier {
            ScrollView {
                VStack(alignment: .leading, spacing: Space.l) {
                    header(dossier.company)
                    if let summary = dossier.company.summary {
                        Text(summary).textSelection(.enabled)
                    }
                    FactGrid {
                        FactRow("Found via", text: dossier.company.foundVia)
                        FactRow("Watched since", text: dossier.watchedSince?.formatted(date: .abbreviated, time: .omitted))
                    }

                    HubSection("Job boards") {
                        if dossier.jobBoards.isEmpty {
                            Text("None stored").foregroundStyle(.secondary)
                        }
                        ForEach(dossier.jobBoards) { board in
                            linkOrText(board.summaryLine, url: board.boardURL)
                        }
                        jobFinderRow(dossier.company)
                    }

                    openThreadsSection(dossier)

                    if let connections = dossier.connections, !connections.isEmpty {
                        HubSection("People you know") {
                            ConnectionList(connections: connections)
                        }
                    }

                    warmPathsSection(dossier)

                    HubSection("People") {
                        if dossier.people.isEmpty {
                            Text("None stored").foregroundStyle(.secondary)
                        }
                        ForEach(dossier.people) { person in
                            PersonRow(
                                person.name, relation: .contact,
                                role: [person.roleTitle, person.relevance.replacingOccurrences(of: "_", with: " ")].compactMap { $0 }.joined(separator: " · ")
                            ) {
                                if let email = person.email {
                                    linkOrText(email, url: "mailto:\(email)")
                                }
                                linkOrText("Source", url: person.sourceURL)
                            }
                        }
                    }
                }
                .padding(Space.l)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        } else {
            ContentUnavailableView("No company selected", systemImage: "building.2")
        }
    }

    /// Starts the job finder on the company, and says how its last run went.
    @ViewBuilder
    private func jobFinderRow(_ company: Company) -> some View {
        HStack(spacing: Space.s) {
            Button("Find jobs", systemImage: "magnifyingglass") { jobFinder.start(companyID: company.id, client: client) }
                .disabled(jobFinder.isRunning(company.id))
                .help("An agent finds the company's open roles: it sets the job board when the hub reads it, or records the roles off the careers page")
            switch jobFinder.states[company.id] {
            case .running:
                ProgressView().controlSize(.small)
                Text("Finding jobs…").foregroundStyle(.secondary)
            case let .finished(openJobs, jobBoard):
                Text(describeFoundJobs(openJobs: openJobs, jobBoard: jobBoard)).foregroundStyle(.secondary)
            case .failed, nil:
                EmptyView()
            }
        }
        if case let .failed(reason) = jobFinder.states[company.id] {
            HubErrorView(HubFailure("Couldn't find the jobs", advice: reason))
        }
    }

    private func describeFoundJobs(openJobs: Int?, jobBoard: String?) -> String {
        let count = openJobs.map { $0 == 1 ? "1 open job" : "\($0) open jobs" } ?? "Done"
        return jobBoard.map { "\(count), read from \($0)" } ?? count
    }

    /// What is going on with the company: its cards on the board and its
    /// latest mail, so its state isn't pieced together from the board and
    /// the inbox. Left out when the hub doesn't send them.
    @ViewBuilder
    private func openThreadsSection(_ dossier: CompanyDossier) -> some View {
        if dossier.applications != nil || dossier.mail != nil {
            HubSection("Applications") {
                if dossier.applicationsOpenFirst.isEmpty {
                    Text("Not on the board").foregroundStyle(.secondary)
                }
                ForEach(dossier.applicationsOpenFirst) { application in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: Space.s) {
                            linkOrText(application.card.jobTitle ?? "Outreach, no posting", url: application.card.jobURL)
                                .bold()
                            Text(application.phaseName).foregroundStyle(.secondary)
                        }
                        if let contactedAt = application.card.application.contactedAt {
                            HeardBackLabel(contactedAt: contactedAt)
                        }
                        if application.phaseIsClosed, let closedReason = application.card.application.closedReason, !closedReason.isEmpty {
                            Label(closedReason, systemImage: SetAside.closed.symbolName)
                                .font(.hubCaption).foregroundStyle(SetAside.closed.tone.color).lineLimit(2)
                        } else if let status = application.card.getFollowUpStatus(now: .now) {
                            ToneChip(status)
                        }
                    }
                    .help(application.card.application.notes ?? "")
                }
                if outreachFailure != nil {
                    HubErrorView($outreachFailure)
                }
                Button("Messaged someone here…", systemImage: "paperplane") {
                    outreachNote = ""
                    isRecordingOutreach = true
                }
                .buttonStyle(.link)
                .help("Puts the company's outreach card in Applied, or counts a follow-up when it's already there, so the hub reminds you a week later")
            }
            .alert("Messaged someone at \(dossier.company.name)", isPresented: $isRecordingOutreach) {
                TextField("Who, and how", text: $outreachNote)
                Button("Record") { recordOutreach(at: dossier.company) }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("Its follow-up falls due a week from today.")
            }

            HubSection("Latest mail") {
                if (dossier.mail ?? []).isEmpty {
                    Text("None matched to this company").foregroundStyle(.secondary)
                }
                ForEach(dossier.mail ?? []) { message in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(message.subject.isEmpty ? "(no subject)" : message.subject).lineLimit(1)
                        Text("\(message.fromLine) · \(message.sentAt.formatted(date: .abbreviated, time: .omitted))")
                            .font(.hubCaption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                if let foldedMailLine = dossier.foldedMailLine {
                    Text(foldedMailLine).font(.hubCaption).foregroundStyle(.secondary)
                }
            }
        }
    }

    /// People who don't work here but can open doors, and a way to add one.
    private func warmPathsSection(_ dossier: CompanyDossier) -> some View {
        HubSection("Can introduce you") {
            ForEach(dossier.warmPaths ?? []) { path in
                PersonRow(
                    path.name, relation: .introducer,
                    detail: [path.note, path.howKnown, path.preferredChannel.map { "prefers \($0)" }].compactMap { $0 }.filter { !$0.isEmpty }
                        .joined(separator: " · ")
                )
                .contextMenu {
                    Button("Remove from \(dossier.company.name)", role: .destructive) { remove(path, from: dossier.company) }
                }
            }
            if failure != nil {
                HubErrorView($failure)
            }
            Button("Add someone who can introduce you", systemImage: "person.badge.plus") { isAddingWarmPath = true }
                .buttonStyle(.link)
        }
        .sheet(isPresented: $isAddingWarmPath) {
            WarmPathSheet(companyName: dossier.company.name) { request in
                do {
                    _ = try await client.send("POST", "v1/companies/\(dossier.company.id)/warm-paths", body: request, as: WarmPath.self)
                    failure = nil
                    onChanged()
                    return true
                } catch {
                    failure = HubFailure("Couldn't add them", error)
                    return false
                }
            }
        }
    }

    private func recordOutreach(at company: Company) {
        let note = outreachNote
        Task {
            do {
                _ = try await client.recordOutreach(companyID: company.id, note: note)
                outreachFailure = nil
                onChanged()
            } catch {
                outreachFailure = HubFailure("Couldn't record the message", error)
            }
        }
    }

    private func remove(_ path: WarmPath, from company: Company) {
        Task {
            do {
                try await client.delete("v1/companies/\(company.id)/warm-paths/\(path.contactID)")
                failure = nil
                onChanged()
            } catch {
                failure = HubFailure("Couldn't remove them", error)
            }
        }
    }

    private func header(_ company: Company) -> some View {
        EntityHeader(eyebrow: "Company", title: company.name) {
            HStack(spacing: Space.m) {
                linkOrText(company.domain, url: company.websiteURL)
                if let careersURL = company.careersURL {
                    linkOrText("Careers", url: careersURL)
                }
                if let country = company.headquartersCountry {
                    Text(country)
                }
                if let size = company.employeeCountRange {
                    Text("\(size) people")
                }
            }
            .tint(.hubAccent)
        }
    }

    @ViewBuilder
    private func linkOrText(_ title: String, url: String?) -> some View {
        if let url, let destination = URL(string: url) {
            Link(title, destination: destination)
        } else {
            Text(title)
        }
    }
}

/// Adds someone the owner knows as a warm path to one company.
struct WarmPathSheet: View {
    let companyName: String
    let onAdd: (AddWarmPathRequest) async -> Bool
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var howKnown = ""
    @State private var preferredChannel = ""
    @State private var note = ""

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("Someone who can introduce you at \(companyName)").font(.hubSection)
            TextField("Name", text: $name, prompt: Text("As you call them; the same name links them to other companies"))
            TextField("How you know them", text: $howKnown, prompt: Text("e.g. a former colleague"))
            TextField("Where to reach them", text: $preferredChannel, prompt: Text("e.g. LinkedIn, WhatsApp"))
            TextField("How they can help here", text: $note, prompt: Text("e.g. interviewed there, knows the CTO"))
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                AsyncButton("Add", busyTitle: "Adding…") {
                    let added = await onAdd(AddWarmPathRequest(name: name, howKnown: howKnown, preferredChannel: preferredChannel, note: note))
                    if added { dismiss() }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
        .padding(Space.xl)
        .frame(width: 480)
    }
}
