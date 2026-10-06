import Foundation
@testable import JobSearchHubCore
import Testing

private let runningOutput = """
gui/501/com.tonypine.jobsearchhub.server = {
\tactive count = 1
\tpath = /Users/ada/Applications/Job Search Hub.app/Contents/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist
\tstate = running

\tprogram = /Users/ada/Applications/Job Search Hub.app/Contents/Helpers/bin/hub-server
\tendpoints = {
\t\tstate = active
\t}
\truns = 1
\tpid = 77032
\tlast exit code = (never exited)
}
"""

@Test func theAgentsStateReadsFromLaunchctl() {
    #expect(ServerLaunchAgent.parseState(runningOutput, status: 0) == .running(pid: 77032))
    let waiting = runningOutput.replacingOccurrences(of: "\tstate = running", with: "\tstate = not running")
        .replacingOccurrences(of: "\tpid = 77032\n", with: "")
        .replacingOccurrences(of: "(never exited)", with: "1")
    #expect(ServerLaunchAgent.parseState(waiting, status: 0) == .waiting(lastExitCode: "1"))
    #expect(ServerLaunchAgent.parseState("Could not find service", status: 113) == .stopped)
}

@Test func eachCommandIsALaunchctlCallInTheUsersDomain() {
    #expect(ServerLaunchAgent.makeArguments(.readState, userID: 501) == ["print", "gui/501/com.tonypine.jobsearchhub.server"])
    #expect(ServerLaunchAgent.makeArguments(.stop, userID: 501) == ["bootout", "gui/501/com.tonypine.jobsearchhub.server"])
    #expect(ServerLaunchAgent.makeArguments(.restart, userID: 501) == ["kickstart", "-k", "gui/501/com.tonypine.jobsearchhub.server"])
}

@Test func theServerAndItsCommandsLiveInTheInstalledBundle() {
    let app = ServerLaunchAgent.makeInstalledAppURL(home: URL(filePath: "/Users/ada"))
    #expect(app.path == "/Users/ada/Applications/Job Search Hub.app")
    #expect(ServerLaunchAgent.makeCommandURL("hub-server", bundle: app).path == "/Users/ada/Applications/Job Search Hub.app/Contents/Helpers/bin/hub-server")
    #expect(ServerLaunchAgent.plistName == "com.tonypine.jobsearchhub.server.plist")
}
