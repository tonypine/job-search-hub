import JobSearchHubCore
import SwiftUI

/// The Profile page's tabs: the profile document agents read, and what
/// sits beside it.
enum ProfileTab: String, CaseIterable, Identifiable {
    case profile
    case knowledgeBase
    case gaps
    case linkedIn
    case answers

    var id: String { rawValue }

    var title: String {
        switch self {
        case .profile: "Profile"
        case .knowledgeBase: "Knowledge base"
        case .gaps: "Gaps"
        case .linkedIn: "LinkedIn"
        case .answers: "Answers"
        }
    }
}

struct ProfilePage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(\.openSettings) private var openSettings
    @AppStorage(SettingsTab.storageKey) private var settingsTab = SettingsTab.connection
    @State private var tab = ProfileTab.profile
    @State private var editor = ProfileEditor()
    @State private var linkedIn: LinkedInProfileResponse?
    @State private var audit = ProfileAudit()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client: client)
                    .task { await editor.load(with: client) }
                    .task { linkedIn = try? await client.get("v1/linkedin/profile", as: LinkedInProfileResponse.self) }
            }
        }
        .navigationTitle("Profile")
    }

    private func content(client: HubClient) -> some View {
        Group {
            switch tab {
            case .profile: profile(client: client)
            case .knowledgeBase: scrolling { KnowledgeBaseSection(client: client) }
            case .gaps: scrolling { MarketGapsSection(client: client) }
            case .linkedIn: linkedInTab(client: client)
            case .answers: scrolling { ApplicationAnswersSection(client: client) }
            }
        }
        .toolbar {
            ToolbarItem(placement: .principal) {
                Picker("Show", selection: $tab) {
                    ForEach(ProfileTab.allCases) { tab in Text(tab.title).tag(tab) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .fixedSize()
                .disabled(editor.isEditing)
            }
            if tab == .profile {
                ToolbarItemGroup {
                    if editor.isEditing {
                        Button("Cancel") { editor.cancelEditing() }
                            .disabled(editor.isSaving)
                        AsyncButton("Save", busyTitle: "Saving…", isBusy: editor.isSaving) { await editor.save(with: client) }
                            .keyboardShortcut("s")
                    } else {
                        Button("Edit", systemImage: "pencil") { editor.startEditing() }
                            .disabled(editor.profile == nil)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func profile(client: HubClient) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            if let error = editor.error {
                HubErrorView(title: editor.isEditing ? "Couldn't save the profile" : "Couldn't load the profile", report: error)
                    .padding(Space.l)
            }
            if editor.isEditing {
                TextEditor(text: $editor.draft)
                    .font(.body.monospaced())
                    .padding(Space.l)
                    .disabled(editor.isSaving)
            } else if let profile = editor.profile {
                scrolling { ProfileDocument(markdown: profile.body) }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    @ViewBuilder
    private func linkedInTab(client: HubClient) -> some View {
        if let linkedIn, !linkedIn.profile.isEmpty {
            scrolling { LinkedInProfileSection(response: linkedIn, audit: audit, client: client) }
        } else {
            ContentUnavailableView {
                Label("No LinkedIn profile yet", systemImage: "person.text.rectangle")
            } description: {
                Text("Import your LinkedIn data export in Settings › Accounts. Agents read the profile in it beside yours.")
            } actions: {
                Button("Open Settings…") {
                    settingsTab = .accounts
                    openSettings()
                }
            }
        }
    }

    /// A tab's content in a readable column.
    private func scrolling<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        ScrollView {
            content()
                .padding(Space.xl)
                .frame(maxWidth: 760, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

struct ProfileDocument: View {
    let markdown: String

    var body: some View {
        if MarkdownBlocks.parse(markdown).isEmpty {
            Text("No profile yet. Edit to write one; agents read it as context for every run.")
                .foregroundStyle(.secondary)
        } else {
            MarkdownDocument(markdown)
        }
    }
}

/// The LinkedIn profile the agents also read, and where it differs from the
/// hub's criteria.
struct LinkedInProfileSection: View {
    let response: LinkedInProfileResponse
    let audit: ProfileAudit
    let client: HubClient

    var body: some View {
        HubSection("From LinkedIn") {
            if !response.criteriaDifferences.isEmpty {
                VStack(alignment: .leading, spacing: Space.s) {
                    Label("LinkedIn and your criteria differ", systemImage: "exclamationmark.triangle.fill").foregroundStyle(Tone.caution.color)
                    ForEach(response.criteriaDifferences, id: \.self) { difference in
                        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                            Text("•")
                            Text(difference)
                        }
                    }
                    Text("Recruiters find you by what LinkedIn says. Edit it there, or your criteria on the Criteria page.")
                        .font(.hubCaption).foregroundStyle(.secondary)
                }
                .hubWell()
            }
            if let headline = response.profile.headline {
                Text(headline).fontWeight(.medium)
            }
            if let positions = response.profile.positions, !positions.isEmpty {
                VStack(alignment: .leading, spacing: Space.xs) {
                    Text("Positions").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
                    ForEach(Array(positions.enumerated()), id: \.offset) { _, position in
                        Text("\(Text("\(position.title) at \(position.company)").fontWeight(.medium))  \(Text(position.period).foregroundStyle(.secondary))")
                    }
                }
            }
            if let skills = response.profile.skills, !skills.isEmpty {
                VStack(alignment: .leading, spacing: Space.xs) {
                    Text("Skills").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
                    Text(skills.joined(separator: " · ")).foregroundStyle(.secondary)
                }
            }
            Text("Agents read this with your profile. Import again from Settings › Accounts to update it.")
                .font(.hubCaption).foregroundStyle(.secondary)
            auditView
        }
        .textSelection(.enabled)
    }

    /// The audit for recruiters: a button, then Claude's suggested edits.
    @ViewBuilder
    private var auditView: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text("Audit for recruiters").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
            switch audit.state {
            case .idle, .auditing:
                AsyncButton("Audit my LinkedIn profile", busyTitle: "Auditing…", systemImage: "wand.and.stars", isBusy: audit.state == .auditing) {
                    await audit.audit(with: client)
                }
                Text("Claude compares your profile with the postings that fit you and what recruiters approached you for, and suggests edits to make on LinkedIn.")
                    .font(.hubCaption).foregroundStyle(.secondary)
            case let .audited(markdown):
                ProfileDocument(markdown: markdown)
                    .hubWell()
                AsyncButton("Audit again", busyTitle: "Auditing…") { await audit.audit(with: client) }
            case let .failed(reason):
                HubErrorView(HubFailure("Couldn't audit the profile", advice: reason)) { Task { await audit.audit(with: client) } }
            }
        }
    }
}

