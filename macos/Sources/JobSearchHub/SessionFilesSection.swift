import AppKit
import JobSearchHubCore
import SwiftUI

/// The one folder Claude sessions in the app can read without /add-dir, so
/// an agent can attach the owner's files, such as the CV, to a form.
struct SessionFilesSection: View {
    @AppStorage(ClaudeLaunch.readableFolderKey) private var readableFolder =
        ClaudeLaunch.getDefaultReadableFolder(home: FileManager.default.homeDirectoryForCurrentUser)

    var body: some View {
        Section("Sessions") {
            LabeledContent("Folder sessions can read") {
                HStack {
                    Text(readableFolder.isEmpty ? "None" : readableFolder).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                    Button("Choose…") { chooseFolder() }
                    if !readableFolder.isEmpty {
                        Button("Clear") { readableFolder = "" }
                    }
                }
            }
            Text("Sessions can attach files from this folder, like your CV, to an application form. Keep it to your job-search documents. It applies to sessions started or resumed after a change.")
                .font(.hubCaption).foregroundStyle(.secondary)
        }
    }

    private func chooseFolder() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        if !readableFolder.isEmpty {
            panel.directoryURL = URL(filePath: readableFolder)
        }
        if panel.runModal() == .OK, let url = panel.url {
            readableFolder = url.path
        }
    }
}
