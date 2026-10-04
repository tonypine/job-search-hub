import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class RecruitersModel {
    private(set) var recruiters: [RecruiterConversation] = []
    private(set) var messages: [LinkedInMessage] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var filter = RecruiterFilter()
    var selectedID: UUID?

    var shownRecruiters: [RecruiterConversation] { filter.apply(to: recruiters) }
    var selected: RecruiterConversation? { recruiters.first { $0.id == selectedID } }

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

    func loadMessages(with client: HubClient) async {
        guard let selectedID else {
            messages = []
            return
        }
        messages = (try? await client.get("v1/linkedin/conversations/\(selectedID.uuidString)/messages", as: ConversationMessagesResponse.self).messages) ?? []
    }
}

/// The recruiters who wrote to the owner on LinkedIn, the latest first,
/// flagged by what their company has open now and whether the owner answered.
struct RecruitersPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var model = RecruitersModel()
    @State private var replyDraft = RecruiterReplyDraft()
    let onOpenCompany: (UUID) -> Void

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                table
                    .task { await model.load(with: client) }
                    .task(id: model.selectedID) { await model.loadMessages(with: client) }
                    .inspector(isPresented: Binding(get: { model.selectedID != nil }, set: { if !$0 { model.selectedID = nil } })) {
                        if let recruiter = model.selected {
                            RecruiterDetail(
                                recruiter: recruiter, messages: model.messages, replyDraft: replyDraft, client: client, onOpenCompany: onOpenCompany
                            )
                                .inspectorColumnWidth(min: 360, ideal: 460, max: 720)
                        }
                    }
                    .toolbar {
                        Toggle("Hiring now", systemImage: "briefcase", isOn: $model.filter.hiringNowOnly)
                            .help("Only recruiters whose company has open jobs in the feed")
                        Toggle("Unanswered", systemImage: "arrowshape.turn.up.left", isOn: $model.filter.unansweredOnly)
                            .help("Only conversations you never answered")
                        Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                            .disabled(model.isLoading)
                    }
            } else {
                NotConnectedView()
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
                    description: Text("Import your LinkedIn archive in Settings › Network. The hub reads the conversations others started and lists the recruiters here.")
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

struct RecruiterDetail: View {
    let recruiter: RecruiterConversation
    let messages: [LinkedInMessage]
    let replyDraft: RecruiterReplyDraft
    let client: HubClient
    let onOpenCompany: (UUID) -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Space.l) {
                EntityHeader(eyebrow: "Recruiter", title: recruiter.startedByName) {
                    VStack(alignment: .leading, spacing: Space.xs) {
                        if let position = recruiter.starterPosition {
                            Text([position, recruiter.starterCompany].compactMap { $0 }.joined(separator: " · "))
                        }
                        HStack(spacing: Space.m) {
                            if let profile = URL(string: recruiter.startedByURL) {
                                Link("LinkedIn profile", destination: profile)
                            }
                            if let companyID = recruiter.companyID {
                                Button("Open company") { onOpenCompany(companyID) }.buttonStyle(.link)
                            }
                        }
                        .tint(.hubAccent)
                    }
                } chips: {
                    if recruiter.isAgency {
                        ToneChip("Agency", tone: .neutral)
                    }
                    if !recruiter.ownerWrote {
                        ToneChip("Unanswered", tone: .caution)
                    }
                }
                FactGrid {
                    FactRow("Company", text: (recruiter.hiringCompany ?? "–") + (recruiter.isAgency ? " (agency)" : ""))
                    FactRow("Role", text: recruiter.role ?? "–")
                    FactRow("Openings", text: recruiter.openingsText.isEmpty ? "None in the feed now" : recruiter.openingsText)
                    FactRow("Answered", text: recruiter.ownerWrote ? "Yes" : "No")
                    FactRow("Why", text: recruiter.classificationReason)
                }
                reply
                HubSection("Conversation") {
                    ForEach(Array(messages.enumerated()), id: \.offset) { entry in
                        VStack(alignment: .leading, spacing: Space.xs) {
                            HStack {
                                Text(entry.element.senderName).fontWeight(.medium)
                                Spacer()
                                Text(entry.element.sentAt.formatted(date: .abbreviated, time: .shortened)).font(.hubCaption).foregroundStyle(.secondary)
                            }
                            Text(entry.element.content).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                        }
                        .hubWell()
                    }
                }
            }
            .padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    /// The draft of a reply, when it is about this recruiter's conversation.
    private var reply: some View {
        HubSection("Reply") {
            let state = replyDraft.conversationID == recruiter.id ? replyDraft.state : .idle
            switch state {
            case .idle, .drafting:
                AsyncButton("Draft a reply", busyTitle: "Drafting…", systemImage: "square.and.pencil", isBusy: state == .drafting) {
                    await replyDraft.draft(conversationID: recruiter.id, client: client)
                }
                Text("Claude drafts a message that picks up from this conversation and names the roles that fit you at their company. You send it yourself.")
                    .font(.hubCaption).foregroundStyle(.secondary)
            case let .drafted(text):
                Text(text).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    .hubWell()
                HStack {
                    Button("Copy", systemImage: "doc.on.doc") { replyDraft.copyDraft() }
                    AsyncButton("Draft again", busyTitle: "Drafting…") { await replyDraft.draft(conversationID: recruiter.id, client: client) }
                }
            case let .failed(reason):
                HubErrorView(HubFailure("Couldn't draft the reply", advice: reason)) {
                    Task { await replyDraft.draft(conversationID: recruiter.id, client: client) }
                }
            }
        }
    }
}
