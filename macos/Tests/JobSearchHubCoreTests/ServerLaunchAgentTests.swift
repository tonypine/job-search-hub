import Foundation
@testable import JobSearchHubCore
import Testing

private let runningOutput = """
gui/501/com.tonypine.jobsearchhub.server = {
\tactive count = 1
\tpath = /Users/ada/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist
\tstate = running

\tprogram = /Users/ada/Library/Application Support/JobSearchHub/bin/run-hub-server
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

@Test func eachActionIsALaunchctlCallInTheUsersDomain() {
    let plist = ServerLaunchAgent.makePlistURL(home: URL(filePath: "/Users/ada"))
    #expect(plist.path == "/Users/ada/Library/LaunchAgents/com.tonypine.jobsearchhub.server.plist")
    #expect(ServerLaunchAgent.makeArguments(.readState, userID: 501, plist: plist) == ["print", "gui/501/com.tonypine.jobsearchhub.server"])
    #expect(ServerLaunchAgent.makeArguments(.start, userID: 501, plist: plist) == ["bootstrap", "gui/501", plist.path])
    #expect(ServerLaunchAgent.makeArguments(.stop, userID: 501, plist: plist) == ["bootout", "gui/501/com.tonypine.jobsearchhub.server"])
    #expect(ServerLaunchAgent.makeArguments(.restart, userID: 501, plist: plist) == ["kickstart", "-k", "gui/501/com.tonypine.jobsearchhub.server"])
}
