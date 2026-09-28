import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class UpdatesModel {
    static let pageSize = 200

    private(set) var days: [UpdateDay] = []
    private(set) var isLoading = false
    private(set) var loadError: String?

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let list = try await client.get(
                "v1/updates", query: [URLQueryItem(name: "limit", value: String(Self.pageSize))], as: HubUpdateList.self
            )
            days = UpdateDay.makeDays(from: list.updates)
            loadError = nil
        } catch {
            loadError = String(describing: error)
        }
    }
}

/// Every update, newest first by day. Clicking one marks it seen and opens
/// its job or company.
struct UpdatesPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @State private var model = UpdatesModel()
    let onOpenJob: (UUID) -> Void
    let onOpenCompany: (UUID) -> Void

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                list(client: client)
                    .task { await model.load(with: client) }
                    .onChange(of: [events.revision, unseen.revision]) { Task { await model.load(with: client) } }
                    .toolbar {
                        Button("Mark all seen", systemImage: "checkmark.circle") {
                            Task { await unseen.markSeen(UpdateSelection(all: true), with: client) }
                        }
                        .disabled(unseen.count == 0)
                    }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Updates")
        .navigationSubtitle(unseen.count == 1 ? "1 unseen" : "\(unseen.count) unseen")
    }

    private func list(client: HubClient) -> some View {
        List {
            ForEach(model.days) { day in
                Section(getDayTitle(day.day)) {
                    ForEach(day.updates) { update in
                        UpdateRow(update: update)
                            .contentShape(Rectangle())
                            .onTapGesture { open(update, with: client) }
                            .contextMenu {
                                if update.isUnseen {
                                    Button("Mark as seen") { Task { await unseen.markSeen(UpdateSelection(ids: [update.id]), with: client) } }
                                }
                                if let sourceURL = update.sourceURL.flatMap(URL.init(string:)) {
                                    Button("Open source") { NSWorkspace.shared.open(sourceURL) }
                                }
                            }
                    }
                }
            }
        }
        .overlay {
            if let loadError = model.loadError {
                ContentUnavailableView("Could not load updates", systemImage: "exclamationmark.triangle", description: Text(loadError))
            } else if model.days.isEmpty && !model.isLoading {
                ContentUnavailableView("No updates", systemImage: "bell", description: Text("Replies, confirmations and other news about applications appear here."))
            }
        }
    }

    private func open(_ update: HubUpdate, with client: HubClient) {
        if update.isUnseen {
            Task { await unseen.markSeen(UpdateSelection(ids: [update.id]), with: client) }
        }
        if let jobID = update.jobID {
            onOpenJob(jobID)
        } else if let companyID = update.companyID {
            onOpenCompany(companyID)
        }
    }

    private func getDayTitle(_ day: Date) -> String {
        let calendar = Calendar.current
        if calendar.isDateInToday(day) { return "Today" }
        if calendar.isDateInYesterday(day) { return "Yesterday" }
        return day.formatted(date: .complete, time: .omitted)
    }
}

struct UpdateRow: View {
    let update: HubUpdate

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            UnseenDot(count: update.isUnseen ? 1 : 0)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline) {
                    Text(update.title).fontWeight(update.isUnseen ? .semibold : .regular)
                    Spacer()
                    Text(update.createdAt.formatted(date: .omitted, time: .shortened))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let subject = update.subject {
                    Text(subject).foregroundStyle(.secondary)
                }
                if let body = update.body, !body.isEmpty {
                    Text(body).foregroundStyle(.secondary).lineLimit(3)
                }
            }
        }
        .padding(.vertical, 4)
    }
}
