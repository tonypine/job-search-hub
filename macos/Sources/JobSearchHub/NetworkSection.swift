import JobSearchHubCore
import SwiftUI
import UniformTypeIdentifiers

@MainActor
@Observable
final class NetworkSectionModel {
    private(set) var summary: ConnectionsSummary?
    private(set) var importLines: [String] = []
    private(set) var isImporting = false
    var errorMessage: String?

    func load(with client: HubClient) async {
        do {
            summary = try await client.get("v1/connections", as: ConnectionsSummary.self)
        } catch {
            errorMessage = String(describing: error)
        }
    }

    /// Imports a Connections.csv, or the export's folder or zip: each file the
    /// hub knows, connections first.
    func importFromLinkedIn(_ picked: URL, with client: HubClient) async {
        isImporting = true
        defer { isImporting = false }
        let isScoped = picked.startAccessingSecurityScopedResource()
        defer { if isScoped { picked.stopAccessingSecurityScopedResource() } }
        importLines = []
        do {
            let files = try listExportFiles(picked)
            var imports = LinkedInArchive.findImports(in: files)
            if imports.isEmpty, picked.pathExtension.lowercased() == "csv" {
                imports = [(.connections, picked)]
            }
            guard !imports.isEmpty else {
                errorMessage = "No file of a LinkedIn export found there, such as Connections.csv or messages.csv."
                return
            }
            for (kind, file) in imports {
                let data = try Data(contentsOf: file)
                switch kind {
                case .connections:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: ConnectionsImport.self).summary)
                case .messages:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: MessagesImport.self).summary)
                case .invitations:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: InvitationsImport.self).summary)
                case .jobApplications:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: LinkedInJobsImport.self)
                        .makeSummary(of: "LinkedIn applications"))
                case .savedJobs:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: LinkedInJobsImport.self)
                        .makeSummary(of: "Saved jobs"))
                }
            }
            errorMessage = nil
            await load(with: client)
        } catch HubError.server(_, let message) {
            errorMessage = message
        } catch {
            errorMessage = String(describing: error)
        }
    }

    /// The files of a picked folder, of a zip once unpacked, or the picked
    /// file itself.
    private func listExportFiles(_ picked: URL) throws -> [URL] {
        var folder = picked
        if picked.pathExtension.lowercased() == "zip" {
            folder = FileManager.default.temporaryDirectory.appending(path: "linkedin-export-\(UUID().uuidString)")
            let unzip = Process()
            unzip.executableURL = URL(fileURLWithPath: "/usr/bin/ditto")
            unzip.arguments = ["-x", "-k", picked.path, folder.path]
            try unzip.run()
            unzip.waitUntilExit()
        }
        guard (try? folder.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) == true,
              let enumerator = FileManager.default.enumerator(at: folder, includingPropertiesForKeys: nil)
        else { return [picked] }
        return enumerator.compactMap { $0 as? URL }
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
            if let summary = model.summary, summary.conversations > 0 || summary.invitations > 0 {
                Text("\(summary.conversations) conversations and \(summary.invitations) invitations from LinkedIn.")
                    .foregroundStyle(.secondary)
            }
            ForEach(model.importLines, id: \.self) { line in
                Text(line).foregroundStyle(.secondary)
            }
            HStack {
                if let errorMessage = model.errorMessage {
                    Text(errorMessage).foregroundStyle(.red)
                }
                Spacer()
                if model.isImporting {
                    ProgressView().controlSize(.small)
                }
                Button("Import from LinkedIn…") { isPickingFile = true }
                    .disabled(model.isImporting)
            }
        } header: {
            Text("Network")
        } footer: {
            Text("Import your LinkedIn data export, its folder or zip, or just its Connections.csv: LinkedIn › Settings › Data privacy › Get a copy of your data. The hub reads your connections, conversations and invitations, keeps them in its own database, and importing again updates them.")
                .foregroundStyle(.secondary)
        }
        .fileImporter(isPresented: $isPickingFile, allowedContentTypes: [.commaSeparatedText, .plainText, .zip, .folder]) { result in
            switch result {
            case let .success(picked): Task { await model.importFromLinkedIn(picked, with: client) }
            case let .failure(error): model.errorMessage = error.localizedDescription
            }
        }
        .task { await model.load(with: client) }
    }
}
