import JobSearchHubCore
import SwiftUI

struct ProfilePage: View {
    @Environment(HubConnection.self) private var connection
    @State private var editor = ProfileEditor()
    @State private var linkedIn: LinkedInProfileResponse?
    @State private var audit = ProfileAudit()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client: client)
                    .task { await editor.load(with: client) }
                    .task { linkedIn = try? await client.get("v1/linkedin/profile", as: LinkedInProfileResponse.self) }
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
                    VStack(alignment: .leading, spacing: 28) {
                        ProfileDocument(markdown: profile.body)
                        if let linkedIn, !linkedIn.profile.isEmpty {
                            LinkedInProfileSection(response: linkedIn, audit: audit, client: client)
                        }
                        ApplicationAnswersSection(client: client)
                    }
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

/// The LinkedIn profile the agents also read, and where it differs from the
/// hub's criteria.
struct LinkedInProfileSection: View {
    let response: LinkedInProfileResponse
    let audit: ProfileAudit
    let client: HubClient

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("From LinkedIn").font(.title3.bold())
            if !response.criteriaDifferences.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Label("LinkedIn and your criteria differ", systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                    ForEach(response.criteriaDifferences, id: \.self) { difference in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Text("•")
                            Text(difference)
                        }
                    }
                    Text("Recruiters find you by what LinkedIn says. Edit it there, or the criteria in Settings.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                .padding(12)
                .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
            }
            if let headline = response.profile.headline {
                Text(headline).fontWeight(.medium)
            }
            if let positions = response.profile.positions, !positions.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Positions").font(.headline)
                    ForEach(Array(positions.enumerated()), id: \.offset) { _, position in
                        Text("\(position.title) at \(position.company)").fontWeight(.medium)
                            + Text("  \(position.period)").foregroundStyle(.secondary)
                    }
                }
            }
            if let skills = response.profile.skills, !skills.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Skills").font(.headline)
                    Text(skills.joined(separator: " · ")).foregroundStyle(.secondary)
                }
            }
            Text("Agents read this with your profile above. Import again from Settings › Network to update it.")
                .font(.caption).foregroundStyle(.secondary)
            auditView
        }
        .textSelection(.enabled)
    }

    /// The audit for recruiters: a button, then Claude's suggested edits.
    @ViewBuilder
    private var auditView: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Audit for recruiters").font(.headline)
            switch audit.state {
            case .idle:
                Button("Audit my LinkedIn profile", systemImage: "wand.and.stars") { Task { await audit.audit(with: client) } }
                Text("Claude compares your profile with the postings that fit you and what recruiters approached you for, and suggests edits to make on LinkedIn.")
                    .font(.caption).foregroundStyle(.secondary)
            case .auditing:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Auditing…").foregroundStyle(.secondary)
                }
            case let .audited(markdown):
                ProfileDocument(markdown: markdown)
                    .padding(12)
                    .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                Button("Audit again") { Task { await audit.audit(with: client) } }
            case let .failed(reason):
                Label(reason, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                Button("Try again") { Task { await audit.audit(with: client) } }
            }
        }
    }
}

