import JobSearchHubCore
import SwiftUI

struct SettingsPage: View {
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
            Section("Status") {
                Label(connection.status.message, systemImage: statusSymbol)
                    .foregroundStyle(connection.status.tone.color)
            }
            if let client = connection.makeClient() {
                ServerSection(client: client)
                SessionFilesSection()
                GoogleSection(client: client)
                NetworkSection(client: client)
                JobCriteriaSection(client: client)
                PipelinePhasesSection(client: client)
                PhonesSection(client: client)
                ModelsSection(client: client)
            }
        }
        .formStyle(.grouped)
        .navigationTitle("Settings")
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
