import AppKit
import JobSearchHubCore
import SwiftUI

/// The one folder Claude sessions in the app can read without /add-dir, so
/// an agent can attach the owner's files, such as the CV, to a form; a
/// status row in Settings › Accounts.
struct SessionFilesSection: View {
    @AppStorage(ClaudeLaunch.readableFolderKey) private var readableFolder =
        ClaudeLaunch.getDefaultReadableFolder(home: FileManager.default.homeDirectoryForCurrentUser)

    var body: some View {
        Section {
            StatusRow(
                "Sessions' folder", symbol: "folder.fill", state: readableFolder.isEmpty ? "None" : "Readable",
                stateTone: readableFolder.isEmpty ? .neutral : .positive,
                detail: readableFolder.isEmpty ? nil : (readableFolder as NSString).abbreviatingWithTildeInPath,
                help: "Sessions can attach files from this folder, like your CV, to an application form. Keep it to your job-search documents. "
                    + "It applies to sessions started or resumed after a change."
            ) {
                if readableFolder.isEmpty {
                    Button("Choose…") { chooseFolder() }
                } else {
                    Menu("Choose…") {
                        Button("Clear") { readableFolder = "" }
                    } primaryAction: {
                        chooseFolder()
                    }
                    .menuStyle(.button)
                    .fixedSize()
                }
            }
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
