import Foundation
import JobSearchHubCore
import Testing

@Test func theBannerWaitsForTheKeychainFirst() {
    #expect(ConnectionProblem.diagnose(isReadingToken: true, hasClient: false, status: .unchecked, isStreamConnected: false) == .waitingForKeychain)
}

@Test func theBannerAsksForSetupWithoutAClient() {
    #expect(ConnectionProblem.diagnose(isReadingToken: false, hasClient: false, status: .unchecked, isStreamConnected: false) == .notSetUp)
    #expect(ConnectionProblem.diagnose(isReadingToken: false, hasClient: true, status: .missingToken, isStreamConnected: false) == .notSetUp)
}

@Test func theBannerSaysWhatTheLastCheckFound() {
    func diagnose(_ status: ConnectionStatus) -> ConnectionProblem? {
        ConnectionProblem.diagnose(isReadingToken: false, hasClient: true, status: status, isStreamConnected: false)
    }
    #expect(diagnose(.serverUnreachable("Could not connect to the server.")) == .unreachable)
    #expect(diagnose(.tokenRefused) == .tokenRefused)
    #expect(diagnose(.failed("decoding")) == .failed("decoding"))
    #expect(diagnose(.connected) == nil)
    #expect(diagnose(.unchecked) == nil)
}

@Test func anOpenEventStreamHidesTheBannerWhateverAnOlderCheckSaid() {
    #expect(ConnectionProblem.diagnose(isReadingToken: false, hasClient: true, status: .serverUnreachable("down"), isStreamConnected: true) == nil)
}

@Test func onlyAnUnreachableHubOffersToStartTheServer() {
    #expect(ConnectionProblem.unreachable.isFixedByStartingTheServer)
    #expect(!ConnectionProblem.tokenRefused.isFixedByStartingTheServer)
    #expect(!ConnectionProblem.notSetUp.isFixedByStartingTheServer)
}

@Test func theBannerNamesTheHubByHostAndPort() {
    #expect(ConnectionProblem.describeAddress(URL(string: "http://localhost:8090"), typed: "http://localhost:8090") == "localhost:8090")
    #expect(ConnectionProblem.describeAddress(URL(string: "https://mac.tailnet.ts.net"), typed: "https://mac.tailnet.ts.net") == "mac.tailnet.ts.net")
    #expect(ConnectionProblem.describeAddress(nil, typed: "not a url") == "not a url")
    #expect(ConnectionProblem.unreachable.describe(hubAddress: "localhost:8090") == "Can't reach the hub at localhost:8090")
}
