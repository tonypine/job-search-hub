import JobSearchHubCore
import SwiftUI

struct ProfilePage: View {
    @Environment(HubConnection.self) private var connection
    @State private var editor = ProfileEditor()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client: client)
                    .task { await editor.load(with: client) }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Profile")
    }

    @ViewBuilder
    private func content(client: HubClient) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            if let errorMessage = editor.errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.orange)
                    .padding()
            }
            if editor.isEditing {
                TextEditor(text: $editor.draft)
                    .font(.body.monospaced())
                    .padding()
                    .disabled(editor.isSaving)
            } else if let profile = editor.profile {
                ScrollView {
                    ProfileDocument(markdown: profile.body)
                        .padding(24)
                        .frame(maxWidth: 760, alignment: .leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .toolbar {
            if editor.isEditing {
                if editor.isSaving {
                    ProgressView().controlSize(.small)
                }
                Button("Cancel") { editor.cancelEditing() }
                    .disabled(editor.isSaving)
                Button("Save") { Task { await editor.save(with: client) } }
                    .keyboardShortcut("s")
                    .disabled(editor.isSaving)
            } else {
                Button("Edit", systemImage: "pencil") { editor.startEditing() }
                    .disabled(editor.profile == nil)
            }
        }
    }
}

struct ProfileDocument: View {
    let markdown: String

    var body: some View {
        let blocks = MarkdownBlocks.parse(markdown)
        VStack(alignment: .leading, spacing: 10) {
            if blocks.isEmpty {
                Text("No profile yet. Edit to write one; agents read it as context for every run.")
                    .foregroundStyle(.secondary)
            }
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, block in
                switch block {
                case .heading(let level, let text):
                    Text(inline(text))
                        .font(level == 1 ? .largeTitle.bold() : .title3.bold())
                        .padding(.top, level == 1 ? 0 : 8)
                case .bullet(let text):
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text("•")
                        Text(inline(text))
                    }
                case .paragraph(let text):
                    Text(inline(text))
                }
            }
        }
        .textSelection(.enabled)
    }

    private func inline(_ text: String) -> AttributedString {
        (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)))
            ?? AttributedString(text)
    }
}
