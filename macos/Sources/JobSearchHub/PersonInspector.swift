import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PersonInspectorModel {
    private(set) var recruiter: RecruiterConversation?
    private(set) var messages: [LinkedInMessage] = []
    /// Their company's open jobs, best screen first; nil until read, or
    /// without a company in the hub.
    private(set) var openJobs: [JobListItem]?
    private(set) var loadError: HubFailure?

    func load(_ conversationID: UUID, with client: HubClient) async {
        do {
            recruiter = try await client.get("v1/recruiters", as: RecruitersResponse.self).recruiters.first { $0.id == conversationID }
            loadError = recruiter == nil ? HubFailure("Couldn't find the recruiter", advice: "Their conversation is no longer in the hub.") : nil
        } catch {
            loadError = HubFailure("Couldn't load the recruiter", error)
        }
        messages = (try? await client.get("v1/linkedin/conversations/\(conversationID.uuidString)/messages", as: ConversationMessagesResponse.self).messages) ?? []
        if let companyID = recruiter?.companyID,
           let jobs = try? await client.get("v1/jobs", query: CompanyJobs.makeQuery(companyID: companyID), as: JobsResponse.self).jobs {
            openJobs = JobsOrder.sort(jobs)
        }
    }
}

/// A person in the inspector, for now a recruiter who wrote on LinkedIn: who
/// they are and what their company has open, then the conversation and a
/// drafted reply.
struct PersonInspector: View {
    let conversationID: UUID
    let client: HubClient
    @Binding var tab: InspectorTab
    @Environment(HubEventStream.self) private var events
    @Environment(RecruiterReplyDraft.self) private var replyDraft
    @Environment(DetailsInspector.self) private var inspector
    @State private var model = PersonInspectorModel()

    var body: some View {
        Group {
            if let recruiter = model.recruiter, recruiter.id == conversationID {
                EntityInspector(tabs: InspectorTab.getTabs(for: .person(conversationID)), tab: $tab) {
                    header(recruiter)
                    actions(recruiter)
                } content: { tab in
                    switch tab {
                    case .conversation: conversationTab(recruiter)
                    default: overviewTab(recruiter)
                    }
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(conversationID, with: client) } }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task(id: conversationID) { await model.load(conversationID, with: client) }
        .onChange(of: events.revision) { Task { await model.load(conversationID, with: client) } }
    }

    private var companyName: String? {
        model.recruiter.flatMap { $0.hiringCompany ?? $0.starterCompany }
    }

    /// Their name and position, and up to three chips: how they relate to
    /// you, whether you answered, and the jobs that fit you at their company.
    private func header(_ recruiter: RecruiterConversation) -> some View {
        let wrote = recruiter.lastMessageAt.map { "wrote \($0.formatted(.relative(presentation: .named))) on LinkedIn" }
        return EntityHeader(
            kind: "Person", parent: companyName,
            openParent: recruiter.companyID.map { id -> () -> Void in { inspector.open(.company(id)) } },
            title: recruiter.startedByName, facts: [recruiter.starterPosition, wrote]
        ) {
            ToneChip(recruiter.isAgency ? "Agency recruiter" : "Recruiter", tone: .neutral)
            if !recruiter.ownerWrote {
                ToneChip("Unanswered", tone: .caution)
            }
            if recruiter.fittingJobs > 0 {
                ToneChip(recruiter.fittingJobs == 1 ? "1 job fits you" : "\(recruiter.fittingJobs) jobs fit you", tone: .positive)
            }
        }
    }

    /// Draft reply, the one primary action, then their LinkedIn profile, with
    /// their company in the overflow.
    private func actions(_ recruiter: RecruiterConversation) -> some View {
        let state = replyDraft.conversationID == recruiter.id ? replyDraft.state : .idle
        return ActionBar {
            AsyncButton("Draft reply", busyTitle: "Drafting…", systemImage: "square.and.pencil", isBusy: state == .drafting) {
                tab = .conversation
                await replyDraft.draft(conversationID: recruiter.id, client: client)
            }
            .help("Claude drafts a message that picks up from this conversation and names the roles that fit you at their company")
        } secondary: {
            if let profile = URL(string: recruiter.startedByURL) {
                Button("Open on LinkedIn", systemImage: "arrow.up.forward.square") { NSWorkspace.shared.open(profile) }
            }
        } overflow: {
            if let companyID = recruiter.companyID {
                Button("Open \(companyName ?? "company")", systemImage: "building.2") { inspector.open(.company(companyID)) }
            }
        }
    }

    // MARK: Overview

    @ViewBuilder
    private func overviewTab(_ recruiter: RecruiterConversation) -> some View {
        FactGrid {
            FactRow("Relation", text: recruiter.isAgency ? "Recruiter at an agency" : "Recruiter")
            FactRow("Hiring for", text: (recruiter.hiringCompany ?? "–") + (recruiter.isAgency ? " (agency)" : ""))
            FactRow("Role", text: recruiter.role ?? "–")
            FactRow("Openings", text: recruiter.openingsText.isEmpty ? "None in the feed now" : recruiter.openingsText)
            FactRow("Answered", text: recruiter.ownerWrote ? "Yes" : "No")
            FactRow("Why", text: recruiter.classificationReason)
        }
        if let companyID = recruiter.companyID {
            HubSection("Open jobs at \(companyName ?? "their company")") {
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
        }
    }

    // MARK: Conversation

    @ViewBuilder
    private func conversationTab(_ recruiter: RecruiterConversation) -> some View {
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
        reply(recruiter)
    }

    /// The draft of a reply, when it is about this recruiter's conversation.
    private func reply(_ recruiter: RecruiterConversation) -> some View {
        HubSection("Drafted reply") {
            let state = replyDraft.conversationID == recruiter.id ? replyDraft.state : .idle
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
                        await replyDraft.draft(conversationID: recruiter.id, client: client)
                    }
                }
                .buttonBorderShape(.capsule)
            case let .failed(reason):
                HubErrorView(HubFailure("Couldn't draft the reply", advice: reason)) {
                    Task { await replyDraft.draft(conversationID: recruiter.id, client: client) }
                }
            }
        }
    }
}
