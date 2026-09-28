import JobSearchHubCore
import SwiftUI
import UniformTypeIdentifiers

@MainActor
@Observable
final class NetworkSectionModel {
    private(set) var summary: ConnectionsSummary?
    private(set) var lastImport: ConnectionsImport?
    private(set) var isImporting = false
    var errorMessage: String?

    func load(with client: HubClient) async {
        do {
            summary = try await client.get("v1/connections", as: ConnectionsSummary.self)
        } catch {
            errorMessage = String(describing: error)
        }
    }

    func importConnections(from file: URL, with client: HubClient) async {
        isImporting = true
        defer { isImporting = false }
        let isScoped = file.startAccessingSecurityScopedResource()
        defer { if isScoped { file.stopAccessingSecurityScopedResource() } }
        do {
            let data = try Data(contentsOf: file)
            lastImport = try await client.upload("v1/connections/import", data: data, contentType: "text/csv", as: ConnectionsImport.self)
            errorMessage = nil
            await load(with: client)
        } catch HubError.server(_, let message) {
            errorMessage = message
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

/// Settings' network: the owner's LinkedIn connections, imported from their
/// own data export, which the hub matches to its companies as warm paths.
struct NetworkSection: View {
    let client: HubClient
    @State private var model = NetworkSectionModel()
    @State private var isPickingFile = false

    var body: some View {
        Section {
            if let summary = model.summary {
                if let lastImportedAt = summary.lastImportedAt {
                    Label(
                        "\(summary.count) connections, \(summary.matched) at companies in the hub. Last imported \(lastImportedAt.formatted(date: .abbreviated, time: .shortened)).",
                        systemImage: "person.2.fill"
                    )
                } else {
                    Label("No connections imported yet.", systemImage: "person.2")
                        .foregroundStyle(.secondary)
                }
            }
            if let lastImport = model.lastImport {
                Text(lastImport.summary).foregroundStyle(.secondary)
            }
            HStack {
                if let errorMessage = model.errorMessage {
                    Text(errorMessage).foregroundStyle(.red)
                }
                Spacer()
                if model.isImporting {
                    ProgressView().controlSize(.small)
                }
                Button("Import connections…") { isPickingFile = true }
                    .disabled(model.isImporting)
            }
        } header: {
            Text("Network")
        } footer: {
            Text("Import Connections.csv from your LinkedIn data export: LinkedIn › Settings › Data privacy › Get a copy of your data › Connections. It stays in the hub's database, and importing again updates it.")
                .foregroundStyle(.secondary)
        }
        .fileImporter(isPresented: $isPickingFile, allowedContentTypes: [.commaSeparatedText, .plainText]) { result in
            switch result {
            case let .success(file): Task { await model.importConnections(from: file, with: client) }
            case let .failure(error): model.errorMessage = error.localizedDescription
            }
        }
        .task { await model.load(with: client) }
    }
}
