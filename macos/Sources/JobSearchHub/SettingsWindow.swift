import JobSearchHubCore
import SwiftUI

/// The Settings window's tabs. The one shown is kept, so the connection
/// banner can open the window on Connection.
enum SettingsTab: String {
    case connection
    case server
    case accounts
    case phones
    case models
    case version

    static let storageKey = "settingsTab"
}

/// The app's Settings window (⌘,): how the app reaches the hub, the server
/// on this Mac, the accounts the hub reads, phones, models, and the app's
/// version. What the search looks for is on the Criteria page.
struct SettingsWindow: View {
    @AppStorage(SettingsTab.storageKey) private var tab = SettingsTab.connection

    var body: some View {
        TabView(selection: $tab) {
            Tab("Connection", systemImage: "network", value: .connection) { ConnectionSettings() }
            Tab("Server", systemImage: "server.rack", value: .server) { ServerSettings() }
            Tab("Accounts", systemImage: "person.crop.circle", value: .accounts) { AccountsSettings() }
            Tab("Phones", systemImage: "iphone", value: .phones) {
                ConnectedSettings { client in PhonesSection(client: client) }
            }
            Tab("Models", systemImage: "cpu", value: .models) {
                ConnectedSettings { client in ModelsSection(client: client) }
            }
            Tab("Version", systemImage: "arrow.down.circle", value: .version) { VersionSettings() }
        }
        .frame(width: 620)
        .frame(minHeight: 420, idealHeight: 560)
    }
}

/// The hub's address and the owner token, and whether they work.
struct ConnectionSettings: View {
    @Environment(HubConnection.self) private var connection
    @State private var tokenField = ""
    @State private var saveFailure: HubFailure?

    var body: some View {
        @Bindable var connection = connection
        Form {
            Section("Hub") {
                TextField("Hub URL", text: $connection.hubURLText, prompt: Text(HubConnection.defaultHubURL))
                SecureField(
                    "Owner token",
                    text: $tokenField,
                    prompt: Text(tokenPrompt)
                )
                HStack {
                    Spacer()
                    Button("Save") { save() }
                        .keyboardShortcut(.defaultAction)
                }
                if saveFailure != nil {
                    HubErrorView($saveFailure)
                }
            }
            Section {
                StatusRow(
                    "Hub", symbol: "network", state: connection.status.title, stateTone: connection.status.tone,
                    detail: connection.status == .connected ? connection.hubURLText : nil,
                    help: "The app reaches the hub at this address with the owner token, which it keeps in the Keychain "
                        + "(a build no Apple team signed keeps it in its preferences when the Keychain refuses it). "
                        + "Settings is for how the app connects; what you search for is on the Criteria page, under You in the sidebar."
                ) {
                    AsyncButton("Test", busyTitle: "Testing…", isBusy: connection.isChecking) { await connection.check() }
                        .help("Test the connection")
                }
                if connection.status != .connected, connection.status != .unchecked {
                    Text(connection.status.message)
                        .font(.hubCaption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
            }
        }
        .formStyle(.grouped)
        .task { await connection.check() }
    }

    private func save() {
        let isNewToken = !tokenField.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        connection.save(newToken: tokenField)
        tokenField = ""
        saveFailure = nil
        if isNewToken, case .unsaved(_, let reason) = connection.token {
            saveFailure = HubFailure(
                "Couldn't keep the token in the Keychain",
                advice: "The app uses it until it quits; save it again after that.",
                details: reason
            )
        }
        Task { await connection.check() }
    }

    private var tokenPrompt: String {
        switch connection.token {
        case .reading: "Waiting for Keychain access"
        case .present:
            switch connection.tokenSource {
            case .keychain: "Saved in the Keychain; enter a new one to replace it"
            case .preferences: "Saved in this build's preferences; enter a new one to replace it"
            case .environment: "From HUB_OWNER_TOKEN in QA mode; enter a new one to replace it"
            }
        case .unsaved: "In use until the app quits; the Keychain refused it"
        case .missing: "HUB_OWNER_TOKEN from the hub's .env"
        }
    }
}

/// The server on this Mac, which starts and stops without a connection.
struct ServerSettings: View {
    var body: some View {
        Form {
            ServerSection()
        }
        .formStyle(.grouped)
    }
}

/// The accounts the hub reads, each a status row: Google, the LinkedIn
/// export, and the folder Claude sessions can read on this Mac.
struct AccountsSettings: View {
    @Environment(HubConnection.self) private var connection

    var body: some View {
        Form {
            if let client = connection.makeClient() {
                GoogleSection(client: client)
                NetworkSection(client: client)
            } else {
                Section {
                    Text("Connect to the hub under Connection to link Google and import from LinkedIn.")
                        .foregroundStyle(.secondary)
                }
            }
            SessionFilesSection()
        }
        .formStyle(.grouped)
    }
}

/// A Settings tab that works through the hub: its form once the app has a
/// client, and a pointer to the Connection tab before.
struct ConnectedSettings<Content: View>: View {
    @Environment(HubConnection.self) private var connection
    @AppStorage(SettingsTab.storageKey) private var tab = SettingsTab.connection
    @ViewBuilder let content: (HubClient) -> Content

    var body: some View {
        if let client = connection.makeClient() {
            Form { content(client) }
                .formStyle(.grouped)
        } else {
            ContentUnavailableView {
                Label("Connect to the hub first", systemImage: "network.slash")
            } description: {
                Text("Set the hub's address and owner token under Connection.")
            } actions: {
                Button("Show Connection") { tab = .connection }
            }
        }
    }
}
