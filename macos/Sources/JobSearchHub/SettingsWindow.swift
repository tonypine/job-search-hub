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
                    Button("Save") { save() }
                        .keyboardShortcut(.defaultAction)
                    AsyncButton("Test connection", busyTitle: "Testing…", isBusy: connection.isChecking) { await connection.check() }
                }
                if saveFailure != nil {
                    HubErrorView($saveFailure)
                }
            }
            Section {
                Label(connection.status.message, systemImage: statusSymbol)
                    .foregroundStyle(connection.status.tone.color)
            } header: {
                Text("Status")
            } footer: {
                Text("Settings is for how the app connects. What you search for is on the Criteria page, under You in the sidebar.")
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .task { await connection.check() }
    }

    private func save() {
        do {
            try connection.save(newToken: tokenField)
            tokenField = ""
            saveFailure = nil
            Task { await connection.check() }
        } catch {
            saveFailure = HubFailure("Couldn't save the token", error)
        }
    }

    private var tokenPrompt: String {
        switch connection.token {
        case .reading: "Waiting for Keychain access"
        case .present: "Saved in the Keychain; enter a new one to replace it"
        case .missing: "HUB_OWNER_TOKEN from the hub's .env"
        }
    }

    private var statusSymbol: String {
        switch connection.status {
        case .connected: "checkmark.circle.fill"
        case .unchecked: "circle.dashed"
        case .waitingForKeychain: "lock"
        default: "exclamationmark.triangle.fill"
        }
    }
}

/// The server on this Mac, which starts and stops without a connection.
struct ServerSettings: View {
    @Environment(HubConnection.self) private var connection

    var body: some View {
        Form {
            ServerSection(client: connection.makeClient())
        }
        .formStyle(.grouped)
    }
}

/// The accounts the hub reads: Google, the LinkedIn export, and the folder
/// Claude sessions can read on this Mac.
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
