import Foundation

/// The hub server's launch agent on this Mac: where it lives in the app's
/// bundle, what launchd is asked to do with it, and how its state reads.
public enum ServerLaunchAgent {
    public static let label = "com.tonypine.jobsearchhub.server"
    /// The agent's plist in the bundle's Contents/Library/LaunchAgents, which
    /// `SMAppService.agent(plistName:)` registers.
    public static let plistName = "\(label).plist"

    /// The app as `install-app.sh`, and later updates, put it: the copy whose
    /// server the agent runs.
    public static func makeInstalledAppURL(home: URL) -> URL {
        home.appending(path: "Applications/Job Search Hub.app")
    }

    /// The command named `name` in the bundle's Helpers/bin, beside the
    /// server: `hub-server`, `hub`, `hub-cvprint` or `hub-update`.
    public static func makeCommandURL(_ name: String, bundle: URL) -> URL {
        bundle.appending(path: "Contents/Helpers/bin/\(name)")
    }

    /// The log the server writes, from HUB_LOG_FILE in the agent's plist.
    public static func makeLogURL(home: URL) -> URL {
        home.appending(path: "Library/Logs/JobSearchHub/server.log")
    }

    /// What launchd is asked to do with the agent. Starting it again once
    /// stopped goes through `SMAppService` instead, which loads it from the
    /// bundle.
    public enum Command: Sendable {
        case readState
        case stop
        case restart
    }

    /// The `launchctl` arguments for the command, in the user's GUI domain.
    public static func makeArguments(_ command: Command, userID: uid_t) -> [String] {
        let service = "gui/\(userID)/\(label)"
        switch command {
        case .readState: return ["print", service]
        case .stop: return ["bootout", service]
        case .restart: return ["kickstart", "-k", service]
        }
    }

    public enum State: Equatable, Sendable {
        /// Loaded and running, with its process id.
        case running(pid: Int)
        /// Loaded but not running, as between a crash and launchd's restart.
        case waiting(lastExitCode: String?)
        /// Not loaded: stopped, or never started.
        case stopped
    }

    /// The state from `launchctl print`'s output; a failed print means the
    /// agent isn't loaded.
    public static func parseState(_ output: String, status: Int32) -> State {
        guard status == 0 else { return .stopped }
        var state: String?
        var pid: Int?
        var lastExitCode: String?
        // The service's own keys come first, one tab in; nested blocks repeat some names deeper.
        for line in output.components(separatedBy: "\n") where line.hasPrefix("\t") && !line.hasPrefix("\t\t") {
            let parts = line.trimmingCharacters(in: .whitespaces).components(separatedBy: " = ")
            guard parts.count == 2 else { continue }
            switch parts[0] {
            case "state": state = state ?? parts[1]
            case "pid": pid = pid ?? Int(parts[1])
            case "last exit code": lastExitCode = lastExitCode ?? parts[1]
            default: break
            }
        }
        if state == "running", let pid {
            return .running(pid: pid)
        }
        return .waiting(lastExitCode: lastExitCode)
    }
}
