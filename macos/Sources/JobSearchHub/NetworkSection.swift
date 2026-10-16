import JobSearchHubCore
import SwiftUI
import UniformTypeIdentifiers

@MainActor
@Observable
final class NetworkSectionModel {
    private(set) var summary: ConnectionsSummary?
    private(set) var importLines: [String] = []
    private(set) var isImporting = false
    var failure: HubFailure?

    func load(with client: HubClient) async {
        do {
            summary = try await client.get("v1/connections", as: ConnectionsSummary.self)
        } catch {
            failure = HubFailure("Couldn't read your connections", error)
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
            let profileFiles = LinkedInArchive.findProfileFiles(in: files)
            if imports.isEmpty, profileFiles.isEmpty, picked.pathExtension.lowercased() == "csv" {
                imports = [(.connections, picked)]
            }
            guard !imports.isEmpty || !profileFiles.isEmpty else {
                failure = HubFailure("Nothing to import", advice: "No file of a LinkedIn export found there, such as Connections.csv or messages.csv.")
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
                case .companyFollows:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: FollowsImport.self).summary)
                case .savedAnswers, .screeningResponses:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: AnswersImport.self).summary)
                case .endorsementsReceived, .endorsementsGiven, .recommendationsReceived, .recommendationsGiven:
                    importLines.append(try await client.upload(kind.importPath, data: data, contentType: "text/csv", as: VouchingImport.self)
                        .makeSummary(of: kind.vouchingTitle))
                }
            }
            if !profileFiles.isEmpty {
                var contents: [String: String] = [:]
                for file in profileFiles {
                    contents[file.lastPathComponent] = try String(contentsOf: file, encoding: .utf8)
                }
                importLines.append(try await client.send("POST", "v1/linkedin/profile/import", body: ProfileImportRequest(files: contents), as: ProfileImportResponse.self).summary)
            }
            failure = nil
            await load(with: client)
        } catch HubError.server(_, let message) {
            failure = HubFailure("The import stopped", advice: message)
        } catch {
            failure = HubFailure("The import stopped", error)
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

/// Settings › Accounts' LinkedIn import, as a status row: the owner's
/// connections, conversations and more from their own data export, which
/// the hub matches to its companies as warm paths.
struct NetworkSection: View {
    let client: HubClient
    @State private var model = NetworkSectionModel()
    @State private var isPickingFile = false

    var body: some View {
        Section {
            StatusRow(
                "LinkedIn import", symbol: "person.2.fill", state: state, stateTone: model.summary?.lastImportedAt == nil ? .neutral : .positive,
                detail: detail,
                help: "Import your LinkedIn data export, its folder or zip, or just its Connections.csv: LinkedIn › Settings › Data privacy › "
                    + "Get a copy of your data. The hub reads your connections, conversations and invitations, keeps them in its own database, "
                    + "and importing again updates them."
            ) {
                AsyncButton("Import…", busyTitle: "Importing…", isBusy: model.isImporting) { isPickingFile = true }
                    .help("Import a LinkedIn data export")
            }
            ForEach(model.importLines, id: \.self) { line in
                Text(line).font(.hubCaption).foregroundStyle(.secondary)
            }
            if model.failure != nil {
                HubErrorView($model.failure)
            }
        }
        .fileImporter(isPresented: $isPickingFile, allowedContentTypes: [.commaSeparatedText, .plainText, .zip, .folder]) { result in
            switch result {
            case let .success(picked): Task { await model.importFromLinkedIn(picked, with: client) }
            case let .failure(error): model.failure = HubFailure("Couldn't open the export", error)
            }
        }
        .task { await model.load(with: client) }
    }

    private var state: String {
        guard let summary = model.summary else { return "Checking…" }
        return summary.lastImportedAt == nil ? "Not imported" : "Imported"
    }

    private var detail: String? {
        guard let summary = model.summary, let lastImportedAt = summary.lastImportedAt else { return nil }
        var parts = ["\(summary.count.formatted()) connections, \(summary.matched.formatted()) at companies here"]
        if summary.conversations > 0 { parts.append("\(summary.conversations.formatted()) conversations") }
        parts.append(lastImportedAt.formatted(date: .abbreviated, time: .omitted))
        return parts.joined(separator: " · ")
    }
}
