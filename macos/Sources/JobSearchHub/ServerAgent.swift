import Foundation
import JobSearchHubCore
import ServiceManagement

enum ServerAgentError: LocalizedError {
    case notInstalledCopy

    var errorDescription: String? {
        "Only the app installed in ~/Applications starts the server. This build would point the agent at itself, and the next build deletes it."
    }
}

/// The server's launch agent, registered with `SMAppService` from the plist
/// in this bundle: launchd runs the server inside the bundle, and System
/// Settings › General › Login Items lists it under the app's name and icon.
enum ServerAgent {
    private static var bundle: URL { Bundle.main.bundleURL }

    /// Whether this build carries the server; one built without Go doesn't.
    static var bundleCarriesServer: Bool {
        FileManager.default.isExecutableFile(atPath: ServerLaunchAgent.makeCommandURL("hub-server", bundle: bundle).path)
    }

    /// Whether this is the installed app, in ~/Applications, rather than a
    /// build elsewhere.
    static var isInstalledCopy: Bool {
        let installed = ServerLaunchAgent.makeInstalledAppURL(home: FileManager.default.homeDirectoryForCurrentUser)
        return bundle.resolvingSymlinksInPath().path == installed.resolvingSymlinksInPath().path
    }

    /// The `hub` command in this bundle, when the build carries one.
    static var hubCommand: URL? {
        let command = ServerLaunchAgent.makeCommandURL("hub", bundle: bundle)
        return FileManager.default.isExecutableFile(atPath: command.path) ? command : nil
    }

    /// Registers the agent when the installed app finds it never was, as
    /// after its first install. One the owner turned off in Login Items stays
    /// off, and a build elsewhere leaves the installed one's agent alone.
    static func registerAtLaunch() {
        guard bundleCarriesServer, isInstalledCopy else { return }
        let service = SMAppService.agent(plistName: ServerLaunchAgent.plistName)
        guard service.status == .notRegistered else { return }
        do {
            try service.register()
        } catch {
            NSLog("Couldn't register the server's launch agent: %@", error.localizedDescription)
        }
    }

    /// Registers the agent anew from this bundle's plist, which loads it and
    /// starts the server: Settings › Server's Start, and `install-app.sh`.
    /// Returns the agent's status, `.requiresApproval` when the owner turned
    /// it off in Login Items and has to allow it there first. Only the
    /// installed app registers it: the agent runs the server from the bundle
    /// that registered it, and one in macos/build/ goes with the next build.
    nonisolated static func register() async throws -> SMAppService.Status {
        guard isInstalledCopy else { throw ServerAgentError.notInstalledCopy }
        let service = SMAppService.agent(plistName: ServerLaunchAgent.plistName)
        if service.status != .notRegistered {
            try? await service.unregister()
        }
        try service.register()
        return service.status
    }

    /// `JobSearchHub --register-server`, which `install-app.sh` runs: registers
    /// the agent, says how it went, and exits before any window opens.
    static func registerFromCommandLine() -> Never {
        Task.detached {
            guard bundleCarriesServer else {
                FileHandle.standardError.write(Data("This build of the app doesn't carry the server.\n".utf8))
                exit(1)
            }
            do {
                if try await register() == .requiresApproval {
                    FileHandle.standardError.write(Data("Allow Job Search Hub in System Settings › General › Login Items, then start the server in its Settings › Server.\n".utf8))
                    SMAppService.openSystemSettingsLoginItems()
                    exit(1)
                }
                exit(0)
            } catch {
                FileHandle.standardError.write(Data("Couldn't register the server's launch agent: \(error.localizedDescription)\n".utf8))
                exit(1)
            }
        }
        dispatchMain()
    }
}
