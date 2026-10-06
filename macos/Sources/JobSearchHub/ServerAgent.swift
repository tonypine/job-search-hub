import Foundation
import JobSearchHubCore

/// The server and the commands this bundle carries. launchd runs the server
/// from the installed copy, through the agent `install-app.sh` writes to
/// ~/Library/LaunchAgents.
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
}
