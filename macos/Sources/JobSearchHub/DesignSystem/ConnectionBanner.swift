import JobSearchHubCore
import SwiftUI

/// The window's one word on the hub being out of reach, above every page:
/// what's wrong, Start server when the server on this Mac is down, and
/// Settings. Pages don't show a not-connected state of their own.
struct ConnectionBanner: View {
    let problem: ConnectionProblem
    @Environment(HubConnection.self) private var connection
    @Environment(\.openSettings) private var openSettings
    @AppStorage(SettingsTab.storageKey) private var settingsTab = SettingsTab.connection
    @State private var server = ServerControl()

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            row
            if server.failure != nil {
                HubErrorView($server.failure)
            }
        }
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .background((problem == .waitingForKeychain ? Tone.neutral : Tone.caution).fill, in: RoundedRectangle(cornerRadius: Radius.card))
        .padding(.horizontal, Space.m)
        .padding(.vertical, Space.s)
        .accessibilityElement(children: .contain)
    }

    private var row: some View {
        HStack(spacing: Space.s) {
            if problem == .waitingForKeychain {
                Image(systemName: "lock").foregroundStyle(Tone.neutral.color).accessibilityHidden(true)
            } else {
                Image(systemName: "network.slash").foregroundStyle(Tone.caution.color).accessibilityHidden(true)
            }
            Text(problem.describe(hubAddress: ConnectionProblem.describeAddress(connection.hubURL, typed: connection.hubURLText)))
                .fontWeight(.medium)
                .help(help)
            Spacer(minLength: Space.s)
            if problem.isFixedByStartingTheServer && server.bundleCarriesServer {
                AsyncButton("Start server", busyTitle: "Starting…", isBusy: server.isWorking) {
                    // A loaded agent that isn't running is kicked; an unloaded one is registered from the bundle.
                    await server.readState()
                    if server.state == .stopped {
                        await server.start()
                    } else {
                        await server.perform(.restart)
                    }
                    try? await Task.sleep(for: .seconds(1))
                    await connection.check()
                }
                .buttonStyle(.link)
            }
            if problem != .waitingForKeychain {
                Button("Settings…") {
                    settingsTab = .connection
                    openSettings()
                }
                .buttonStyle(.link)
            }
        }
    }

    private var help: String {
        switch problem {
        case .waitingForKeychain: "Allow Job Search Hub to read its owner token when the Keychain asks."
        case let .failed(reason): reason
        default: ""
        }
    }
}

/// What the window shows in place of a page before the app has a hub
/// address and owner token, under the banner that says the same.
struct ConnectToHubView: View {
    @Environment(\.openSettings) private var openSettings
    @AppStorage(SettingsTab.storageKey) private var settingsTab = SettingsTab.connection

    var body: some View {
        ContentUnavailableView {
            Label("Connect to your hub", systemImage: "network.slash")
        } description: {
            Text("Set the hub's address and owner token in Settings, under Connection.")
        } actions: {
            Button("Settings…") {
                settingsTab = .connection
                openSettings()
            }
        }
    }
}
