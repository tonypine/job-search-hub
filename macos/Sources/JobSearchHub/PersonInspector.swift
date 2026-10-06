import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PersonInspectorModel {
    private(set) var person: RelatedPerson?
    /// The others at their company, from the same list.
    private(set) var colleagues: [RelatedPerson] = []
    private(set) var messages: [LinkedInMessage] = []
    /// Their company's open jobs, best screen first; nil until read, or
    /// without a company in the hub.
    private(set) var openJobs: [JobListItem]?
    private(set) var loadError: HubFailure?

    func load(_ reference: PersonReference, with client: HubClient) async {
        do {
            let people = try await client.get("v1/people", query: PeopleQuery.make(companyID: reference.companyID), as: PeopleResponse.self).people
            person = people.first { $0.key == reference.key }
            colleagues = reference.companyID == nil ? [] : people.filter { $0.key != reference.key }
            loadError = person == nil ? HubFailure("Couldn't find them", advice: "They're no longer in the hub.") : nil
        } catch {
            loadError = HubFailure("Couldn't load the person", error)
        }
        if let conversationID = person?.conversationID {
            messages = (try? await client.get("v1/linkedin/conversations/\(conversationID.uuidString)/messages", as: ConversationMessagesResponse.self).messages) ?? []
        }
        if let companyID = person?.companyID,
           let jobs = try? await client.get("v1/jobs", query: CompanyJobs.makeQuery(companyID: companyID), as: JobsResponse.self).jobs {
            openJobs = JobsOrder.sort(jobs)
        }
    }
}

/// A person in the inspector, whatever their relation: who they are and
/// what they can do for you, the jobs that fit you at their company and who
/// else is there; then, for a recruiter, the conversation and a drafted
/// reply.
struct PersonInspector: View {
    let reference: PersonReference
    let client: HubClient
    @Binding var tab: InspectorTab
    @Environment(HubEventStream.self) private var events
    @Environment(RecruiterReplyDraft.self) private var replyDraft
    @Environment(DetailsInspector.self) private var inspector
    @State private var model = PersonInspectorModel()

    var body: some View {
        Group {
            if let person = model.person, person.key == reference.key {
                EntityInspector(subject: .person(reference), tabs: InspectorTab.getTabs(for: .person(reference)), tab: $tab) {
                    header(person)
                    actions(person)
                } content: { tab in
                    switch tab {
                    case .conversation: conversationTab(person)
                    default: overviewTab(person)
                    }
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(reference, with: client) } }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task(id: reference) { await model.load(reference, with: client) }
        .onChange(of: events.revision) { Task { await model.load(reference, with: client) } }
    }

    /// Their name and role, and up to three chips: how they relate to you,
    /// whether you answered, and the jobs that fit you at their company.
    private func header(_ person: RelatedPerson) -> some View {
        let wrote = person.lastContactAt.map { "last message \($0.formatted(.relative(presentation: .named))) on LinkedIn" }
        return EntityHeader(
            kind: "Person", parent: person.companyName,
            openParent: person.companyID.map { id -> () -> Void in { inspector.open(.company(id)) } },
            title: person.name, facts: [person.role, wrote]
        ) {
            ToneChip(person.relationTitle, tone: .neutral)
            if person.isUnanswered {
                ToneChip("Unanswered", tone: .caution)
            }
            if person.fittingJobs > 0 {
                ToneChip(person.fittingJobs == 1 ? "1 job fits you" : "\(person.fittingJobs) jobs fit you", tone: .positive)
            }
        }
    }

    /// The one primary action is the next step for the relation: Draft reply
    /// for a recruiter, else reaching them on LinkedIn or by mail.
    private func actions(_ person: RelatedPerson) -> some View {
        let profile = person.profileURL.flatMap(URL.init(string:))
        let mail = person.email.flatMap { URL(string: "mailto:\($0)") }
        return ActionBar {
            if let conversationID = person.conversationID {
                let state = replyDraft.conversationID == conversationID ? replyDraft.state : .idle
                AsyncButton("Draft reply", busyTitle: "Drafting…", systemImage: "square.and.pencil", isBusy: state == .drafting) {
                    tab = .conversation
                    await replyDraft.draft(conversationID: conversationID, client: client)
                }
                .help("Claude drafts a message that picks up from this conversation and names the roles that fit you at their company")
            } else if let profile {
                Button("Open on LinkedIn", systemImage: "arrow.up.forward.square") { NSWorkspace.shared.open(profile) }
            } else if let mail {
                Button("Email", systemImage: "envelope") { NSWorkspace.shared.open(mail) }
            }
        } secondary: {
            if person.conversationID != nil, let profile {
                Button("Open on LinkedIn", systemImage: "arrow.up.forward.square") { NSWorkspace.shared.open(profile) }
            } else if profile != nil, let mail {
                Button("Email", systemImage: "envelope") { NSWorkspace.shared.open(mail) }
            }
        } overflow: {
            if let companyID = person.companyID {
                Button("Open \(person.companyName ?? "company")", systemImage: "building.2") { inspector.open(.company(companyID)) }
            }
            if let source = person.sourceURL.flatMap(URL.init(string:)) {
                Button("Open where they were found", systemImage: "safari") { NSWorkspace.shared.open(source) }
            }
        }
    }

    // MARK: Overview

    @ViewBuilder
    private func overviewTab(_ person: RelatedPerson) -> some View {
        HubSection("What they can do for you") {
            Text(person.whatTheyCanDo).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
            FactGrid {
                FactRow("Relation", text: person.relationTitle)
                FactRow("Company", text: person.companyName)
                FactRow("Role", text: person.role)
                FactRow("Hiring for", text: person.hiringRole)
                FactRow("Relevance", text: person.relevanceTitle)
                FactRow("How you know them", text: person.howKnown)
                FactRow("Reach them on", text: person.preferredChannel)
                FactRow("Closeness", text: person.closeness)
                FactRow("Source", text: describeSource(person))
                FactRow("Last message", text: person.lastContactAt.map { $0.formatted(date: .abbreviated, time: .omitted) })
                FactRow("Answered", text: person.answered.map { $0 ? "Yes" : "No" })
                FactRow("Email", text: person.email)
                FactRow("Why", text: person.relation == .recruiter ? person.note : nil)
                FactRow("Notes", text: person.relation == .contact ? person.note : nil)
            }
        }
        if let companyID = person.companyID {
            HubSection("Jobs at \(person.companyName ?? "their company")") {
                if let openJobs = model.openJobs {
                    if openJobs.isEmpty {
                        Text("None in the feed now").foregroundStyle(.secondary)
                    }
                    CompanyJobRows(jobs: openJobs)
                } else {
                    ProgressView().controlSize(.small)
                }
            } trailing: {
                Button("Company") { inspector.open(.company(companyID)) }
                    .buttonStyle(.link)
            }
            if !model.colleagues.isEmpty {
                HubSection("Also at \(person.companyName ?? "their company")") {
                    ForEach(model.colleagues) { colleague in
                        PersonLinkRow(person: colleague)
                    }
                } trailing: {
                    Button("Company") { inspector.open(.company(companyID), tab: .people) }
                        .buttonStyle(.link)
                }
            }
        }
    }

    private func describeSource(_ person: RelatedPerson) -> String {
        switch person.relation {
        case .contact: "Company research"
        case .connection: "LinkedIn connection"
        case .introducer: "Added on the company"
        case .recruiter: "LinkedIn conversation"
        }
    }

    // MARK: Conversation

    @ViewBuilder
    private func conversationTab(_ person: RelatedPerson) -> some View {
        if let conversationID = person.conversationID {
            HubSection("Conversation") {
                if model.messages.isEmpty {
                    Text("No messages read").foregroundStyle(.secondary)
                }
                ForEach(Array(model.messages.enumerated()), id: \.offset) { entry in
                    VStack(alignment: .leading, spacing: Space.xs) {
                        Text(entry.element.content).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                            .hubWell()
                        Text("\(entry.element.senderName) · \(entry.element.sentAt.formatted(date: .abbreviated, time: .shortened))")
                            .font(.hubCaption).foregroundStyle(.secondary)
                    }
                }
            }
            reply(conversationID)
        } else {
            HubSection("Conversation") {
                Text("The hub keeps the LinkedIn conversations recruiters start, so there's none with \(person.name) here. Reach them on LinkedIn or by mail.")
                    .foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    /// The draft of a reply, when it is about this recruiter's conversation.
    private func reply(_ conversationID: UUID) -> some View {
        HubSection("Drafted reply") {
            let state = replyDraft.conversationID == conversationID ? replyDraft.state : .idle
            switch state {
            case .idle:
                Text("Draft reply has Claude write a message that picks up from this conversation and names the roles that fit you at their company. You send it yourself.")
                    .font(.hubCaption).foregroundStyle(.secondary)
            case .drafting:
                HStack(spacing: Space.s) {
                    ProgressView().controlSize(.small)
                    Text("Drafting…").foregroundStyle(.secondary)
                }
            case let .drafted(text):
                Text(text).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    .hubWell()
                HStack {
                    Button("Copy", systemImage: "doc.on.doc") { replyDraft.copyDraft() }
                    AsyncButton("Redraft", busyTitle: "Drafting…", systemImage: "arrow.clockwise") {
                        await replyDraft.draft(conversationID: conversationID, client: client)
                    }
                }
                .buttonBorderShape(.capsule)
            case let .failed(reason):
                HubErrorView(HubFailure("Couldn't draft the reply", advice: reason)) {
                    Task { await replyDraft.draft(conversationID: conversationID, client: client) }
                }
            }
        }
    }
}
