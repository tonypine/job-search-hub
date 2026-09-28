import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class RecruitersModel {
    private(set) var recruiters: [RecruiterConversation] = []
    private(set) var messages: [LinkedInMessage] = []
    private(set) var isLoading = false
    private(set) var loadError: String?
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
            loadError = String(describing: error)
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
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
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
                HStack(spacing: 6) {
                    Text(recruiter.hiringCompany ?? "–")
                    if recruiter.isAgency {
                        Text("Agency").font(.caption2).padding(.horizontal, 5).padding(.vertical, 1)
                            .background(.quinary, in: Capsule())
                    }
                }
            }
            TableColumn("Role") { recruiter in Text(recruiter.role ?? "").help(recruiter.role ?? "") }
            TableColumn("Openings") { recruiter in
                Text(recruiter.openingsText).fontWeight(recruiter.fittingJobs > 0 ? .semibold : .regular)
                    .foregroundStyle(recruiter.fittingJobs > 0 ? AnyShapeStyle(.green) : AnyShapeStyle(.primary))
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
                ContentUnavailableView("Could not load recruiters", systemImage: "exclamationmark.triangle", description: Text(loadError))
            } else if model.recruiters.isEmpty && !model.isLoading {
                ContentUnavailableView(
                    "No recruiters yet", systemImage: "person.crop.rectangle.stack",
                    description: Text("Import your LinkedIn archive in Settings › Network. The hub reads the conversations others started and lists the recruiters here.")
                )
            }
        }
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
            VStack(alignment: .leading, spacing: 16) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(recruiter.startedByName).font(.title2.weight(.semibold))
                    if let position = recruiter.starterPosition {
                        Text([position, recruiter.starterCompany].compactMap { $0 }.joined(separator: " · ")).foregroundStyle(.secondary)
                    }
                    HStack(spacing: 12) {
                        if let profile = URL(string: recruiter.startedByURL) {
                            Link("LinkedIn profile", destination: profile)
                        }
                        if let companyID = recruiter.companyID {
                            Button("Open company") { onOpenCompany(companyID) }.buttonStyle(.link)
                        }
                    }
                }
                Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 6) {
                    row("Company", (recruiter.hiringCompany ?? "–") + (recruiter.isAgency ? " (agency)" : ""))
                    row("Role", recruiter.role ?? "–")
                    row("Openings", recruiter.openingsText.isEmpty ? "None in the feed now" : recruiter.openingsText)
                    row("Answered", recruiter.ownerWrote ? "Yes" : "No")
                    if let reason = recruiter.classificationReason {
                        row("Why", reason)
                    }
                }
                reply
                VStack(alignment: .leading, spacing: 12) {
                    Text("Conversation").font(.headline)
                    ForEach(Array(messages.enumerated()), id: \.offset) { entry in
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(entry.element.senderName).fontWeight(.medium)
                                Spacer()
                                Text(entry.element.sentAt.formatted(date: .abbreviated, time: .shortened)).font(.caption).foregroundStyle(.secondary)
                            }
                            Text(entry.element.content).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(10)
                        .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                    }
                }
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    /// The draft of a reply, when it is about this recruiter's conversation.
    private var reply: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Reply").font(.headline)
            let state = replyDraft.conversationID == recruiter.id ? replyDraft.state : .idle
            switch state {
            case .idle:
                Button("Draft a reply", systemImage: "square.and.pencil") {
                    Task { await replyDraft.draft(conversationID: recruiter.id, client: client) }
                }
                Text("Claude drafts a message that picks up from this conversation and names the roles that fit you at their company. You send it yourself.")
                    .font(.caption).foregroundStyle(.secondary)
            case .drafting:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Drafting…").foregroundStyle(.secondary)
                }
            case let .drafted(text):
                Text(text).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    .padding(10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                HStack {
                    Button("Copy", systemImage: "doc.on.doc") { replyDraft.copyDraft() }
                    Button("Draft again") { Task { await replyDraft.draft(conversationID: recruiter.id, client: client) } }
                }
            case let .failed(reason):
                Label(reason, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                Button("Try again") { Task { await replyDraft.draft(conversationID: recruiter.id, client: client) } }
            }
        }
    }

    private func row(_ label: String, _ value: String) -> some View {
        GridRow {
            Text(label).foregroundStyle(.secondary).gridColumnAlignment(.trailing)
            Text(value).textSelection(.enabled)
        }
    }
}
