import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompanyInspectorModel {
    private(set) var dossier: CompanyDossier?
    /// Its open jobs, best screen first; nil until read.
    private(set) var openJobs: [JobListItem]?
    /// Everyone who can get you in there; nil until read.
    private(set) var people: [RelatedPerson]?
    private(set) var loadError: HubFailure?

    func load(_ companyID: UUID, with client: HubClient) async {
        do {
            dossier = try await client.get("v1/companies/\(companyID.uuidString)", as: CompanyDossier.self)
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the company", error)
        }
        if let jobs = try? await client.get("v1/jobs", query: CompanyJobs.makeQuery(companyID: companyID), as: JobsResponse.self).jobs {
            openJobs = JobsOrder.sort(jobs)
        }
        if let listed = try? await client.get("v1/people", query: PeopleQuery.make(companyID: companyID), as: PeopleResponse.self).people {
            people = listed
        }
    }
}

/// One company in the inspector: its header and actions, then Overview (the
/// summary, boards, applications and latest mail), Jobs (its open jobs),
/// People (everyone who can get you in) and its Session.
struct CompanyInspector: View {
    let companyID: UUID
    let client: HubClient
    @Binding var tab: InspectorTab
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(CompanyResearch.self) private var research
    @Environment(CompanyJobFinder.self) private var jobFinder
    @Environment(DetailsInspector.self) private var inspector
    @State private var model = CompanyInspectorModel()
    @State private var isAddingWarmPath = false
    @State private var failure: HubFailure?
    @State private var isRecordingOutreach = false
    @State private var outreachNote = ""
    @State private var outreachFailure: HubFailure?

    var body: some View {
        Group {
            if let dossier = model.dossier, dossier.company.id == companyID {
                EntityInspector(tabs: InspectorTab.getTabs(for: .company(companyID)), tab: $tab) {
                    header(dossier)
                    actions(dossier)
                } content: { tab in
                    switch tab {
                    case .jobs: jobsTab(dossier)
                    case .people: peopleTab(dossier)
                    case .session: InspectorSessionTab(subject: .company(companyID), client: client)
                    default: overviewTab(dossier)
                    }
                }
                .alert("Messaged someone at \(dossier.company.name)", isPresented: $isRecordingOutreach) {
                    TextField("Who, and how", text: $outreachNote)
                    Button("Record") { recordOutreach(at: dossier.company) }
                    Button("Cancel", role: .cancel) {}
                } message: {
                    Text("Its follow-up falls due a week from today.")
                }
                .sheet(isPresented: $isAddingWarmPath) {
                    WarmPathSheet(companyName: dossier.company.name) { request in
                        do {
                            _ = try await client.send("POST", "v1/companies/\(companyID)/warm-paths", body: request, as: WarmPath.self)
                            failure = nil
                            await model.load(companyID, with: client)
                            return true
                        } catch {
                            failure = HubFailure("Couldn't add them", error)
                            return false
                        }
                    }
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(companyID, with: client) } }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task(id: companyID) {
            await model.load(companyID, with: client)
            await unseen.markSeen(UpdateSelection(companyID: companyID), with: client)
        }
        .onChange(of: [events.revision, research.revision, jobFinder.revision]) { Task { await model.load(companyID, with: client) } }
    }

    /// The company's name and size, and up to three chips: watched, its open
    /// jobs, and its most pressing follow-up.
    private func header(_ dossier: CompanyDossier) -> some View {
        let company = dossier.company
        return EntityHeader(
            kind: "Company", title: company.name,
            facts: [company.domain, company.employeeCountRange.map { "\($0) people" }, company.headquartersCountry]
        ) {
            if dossier.watchedSince != nil {
                ToneChip("Watching", tone: .accent)
            }
            if let openJobs = model.openJobs {
                ToneChip(openJobs.count == 1 ? "1 open job" : "\(openJobs.count) open jobs", tone: .neutral)
            }
            if let followUp = getMostPressingFollowUp(dossier) {
                ToneChip(followUp)
            }
        }
    }

    private func getMostPressingFollowUp(_ dossier: CompanyDossier) -> FollowUpStatus? {
        dossier.applicationsOpenFirst
            .filter { !$0.phaseIsClosed }
            .compactMap { application in application.card.followUpDueAt.map { ($0, application.card) } }
            .min { $0.0 < $1.0 }?
            .1.getFollowUpStatus(now: .now)
    }

    /// Find jobs, the one primary action, then Messaged someone…, with its
    /// site and careers page in the overflow.
    private func actions(_ dossier: CompanyDossier) -> some View {
        let company = dossier.company
        return ActionBar {
            Button("Find jobs", systemImage: "magnifyingglass") { jobFinder.start(companyID: company.id, client: client) }
                .disabled(jobFinder.isRunning(company.id))
                .help("An agent finds the company's open roles: it sets the job board when the hub reads it, or records the roles off the careers page")
        } secondary: {
            Button("Messaged someone…", systemImage: "paperplane") {
                outreachNote = ""
                isRecordingOutreach = true
            }
            .help("Puts the company's outreach card in Applied, or counts a follow-up when it's already there, so the hub reminds you a week later")
        } overflow: {
            if let website = company.websiteURL.flatMap(URL.init(string:)) {
                Button("Open website", systemImage: "safari") { NSWorkspace.shared.open(website) }
            }
            if let careers = company.careersURL.flatMap(URL.init(string:)) {
                Button("Open careers page", systemImage: "briefcase") { NSWorkspace.shared.open(careers) }
            }
            Button("Add someone who can introduce you…", systemImage: "person.badge.plus") { isAddingWarmPath = true }
        }
    }

    // MARK: Overview

    @ViewBuilder
    private func overviewTab(_ dossier: CompanyDossier) -> some View {
        if let summary = dossier.company.summary {
            Text(summary).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
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
            jobFinderState(dossier.company)
        }
        applicationsSection(dossier)
        mailSection(dossier)
    }

    /// How the job finder's last run on the company went.
    @ViewBuilder
    private func jobFinderState(_ company: Company) -> some View {
        switch jobFinder.states[company.id] {
        case .running:
            HStack(spacing: Space.s) {
                ProgressView().controlSize(.small)
                Text("Finding jobs…").foregroundStyle(.secondary)
            }
        case let .finished(openJobs, jobBoard):
            Text(describeFoundJobs(openJobs: openJobs, jobBoard: jobBoard)).foregroundStyle(.secondary)
        case let .failed(reason):
            HubErrorView(HubFailure("Couldn't find the jobs", advice: reason))
        case nil:
            EmptyView()
        }
    }

    private func describeFoundJobs(openJobs: Int?, jobBoard: String?) -> String {
        let count = openJobs.map { $0 == 1 ? "1 open job" : "\($0) open jobs" } ?? "Done"
        return jobBoard.map { "\(count), read from \($0)" } ?? count
    }

    /// Its cards on the board, so its state isn't pieced together from the
    /// board and the inbox. Left out when the hub doesn't send them.
    @ViewBuilder
    private func applicationsSection(_ dossier: CompanyDossier) -> some View {
        if dossier.applications != nil || dossier.mail != nil {
            HubSection("Applications") {
                if dossier.applicationsOpenFirst.isEmpty {
                    Text("Not on the board").foregroundStyle(.secondary)
                }
                ForEach(dossier.applicationsOpenFirst) { application in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: Space.s) {
                            if let jobID = application.card.application.jobID {
                                Button(application.card.jobTitle ?? "Job") { inspector.open(.job(jobID)) }
                                    .buttonStyle(.link)
                                    .bold()
                            } else {
                                Text(application.card.jobTitle ?? "Outreach, no posting").bold()
                            }
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
            }
        }
    }

    @ViewBuilder
    private func mailSection(_ dossier: CompanyDossier) -> some View {
        if dossier.mail != nil {
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

    // MARK: Jobs

    private func jobsTab(_ dossier: CompanyDossier) -> some View {
        HubSection("Open jobs") {
            if let openJobs = model.openJobs {
                if openJobs.isEmpty {
                    Text("None in the feed. Find jobs looks for its board or careers page.").foregroundStyle(.secondary)
                }
                CompanyJobRows(jobs: openJobs)
            } else {
                ProgressView().controlSize(.small)
            }
        }
    }

    // MARK: People

    /// The people list narrowed to the company, each opening in the
    /// inspector, and a way to add someone who can introduce you.
    private func peopleTab(_ dossier: CompanyDossier) -> some View {
        HubSection("People") {
            if let people = model.people {
                if people.isEmpty {
                    Text("No one yet. Research the company, or add someone who can introduce you.").foregroundStyle(.secondary)
                }
                ForEach(people) { person in
                    PersonLinkRow(person: person)
                        .contextMenu {
                            if person.relation == .introducer {
                                Button("Remove from \(dossier.company.name)", role: .destructive) { remove(person, from: dossier.company) }
                            }
                        }
                    if person.id != people.last?.id {
                        Divider()
                    }
                }
            } else {
                ProgressView().controlSize(.small)
            }
            if failure != nil {
                HubErrorView($failure)
            }
            Button("Add someone who can introduce you", systemImage: "person.badge.plus") { isAddingWarmPath = true }
                .buttonStyle(.link)
        }
    }

    private func recordOutreach(at company: Company) {
        let note = outreachNote
        Task {
            do {
                _ = try await client.recordOutreach(companyID: company.id, note: note)
                outreachFailure = nil
                await model.load(companyID, with: client)
            } catch {
                outreachFailure = HubFailure("Couldn't record the message", error)
            }
        }
    }

    /// Unlinks an introducer from the company.
    private func remove(_ introducer: RelatedPerson, from company: Company) {
        Task {
            do {
                try await client.delete("v1/companies/\(company.id)/warm-paths/\(introducer.personID)")
                failure = nil
                await model.load(companyID, with: client)
            } catch {
                failure = HubFailure("Couldn't remove them", error)
            }
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

/// A company's open jobs, each opening in the inspector: its title, where
/// and when it was posted, and its screen.
struct CompanyJobRows: View {
    let jobs: [JobListItem]
    @Environment(DetailsInspector.self) private var inspector

    var body: some View {
        ForEach(jobs) { item in
            InspectorLinkRow(item.job.title, detail: describe(item.job)) {
                ToneChip(screen: item.fit.level)
            } open: {
                inspector.open(.job(item.job.id))
            }
            if item.id != jobs.last?.id {
                Divider()
            }
        }
    }

    private func describe(_ job: Job) -> String {
        let posted = "posted \((job.publishedAt ?? job.firstSeenAt).formatted(.relative(presentation: .named)))"
        return [job.location, posted].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
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
