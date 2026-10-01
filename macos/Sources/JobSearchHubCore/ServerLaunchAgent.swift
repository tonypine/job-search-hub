import Foundation

/// The hub server's launch agent on this Mac: what launchd is asked to do
/// with it, and how its state reads.
public enum ServerLaunchAgent {
    public static let label = "com.tonypine.jobsearchhub.server"

    public static func makePlistURL(home: URL) -> URL {
        home.appending(path: "Library/LaunchAgents/\(label).plist")
    }

    public static func makeLogURL(home: URL) -> URL {
        home.appending(path: "Library/Logs/JobSearchHub/server.log")
    }

    public enum Action: Sendable {
        case readState
        case start
        case stop
        case restart
    }

    /// The `launchctl` arguments for the action, in the user's GUI domain.
    public static func makeArguments(_ action: Action, userID: uid_t, plist: URL) -> [String] {
        let domain = "gui/\(userID)"
        switch action {
        case .readState: return ["print", "\(domain)/\(label)"]
        case .start: return ["bootstrap", domain, plist.path]
        case .stop: return ["bootout", "\(domain)/\(label)"]
        case .restart: return ["kickstart", "-k", "\(domain)/\(label)"]
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
