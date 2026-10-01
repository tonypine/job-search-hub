import JobSearchHubCore
import SwiftUI

struct SettingsPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var tokenField = ""
    @State private var saveError: String?

    var body: some View {
        @Bindable var connection = connection
        Form {
            Section("Hub") {
                TextField("Hub URL", text: $connection.hubURLText, prompt: Text(HubConnection.defaultHubURL))
                SecureField(
                    "Owner token",
                    text: $tokenField,
                    prompt: Text(connection.hasToken ? "Saved in the Keychain; enter a new one to replace it" : "HUB_OWNER_TOKEN from the hub's .env")
                )
                HStack {
                    Button("Save") { save() }
                        .keyboardShortcut(.defaultAction)
                    Button("Test connection") { Task { await connection.check() } }
                        .disabled(connection.isChecking)
                    if connection.isChecking {
                        ProgressView().controlSize(.small)
                    }
                }
                if let saveError {
                    Text(saveError).foregroundStyle(.red)
                }
            }
            Section("Status") {
                Label(connection.status.message, systemImage: statusSymbol)
                    .foregroundStyle(statusColor)
            }
            if let client = connection.makeClient() {
                ServerSection(client: client)
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
            saveError = nil
            Task { await connection.check() }
        } catch {
            saveError = String(describing: error)
        }
    }

    private var statusSymbol: String {
        switch connection.status {
        case .connected: "checkmark.circle.fill"
        case .unchecked: "circle.dashed"
        default: "exclamationmark.triangle.fill"
        }
    }

    private var statusColor: Color {
        switch connection.status {
        case .connected: .green
        case .unchecked: .secondary
        default: .orange
        }
    }
}
