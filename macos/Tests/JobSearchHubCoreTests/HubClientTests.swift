import Foundation
@testable import JobSearchHubCore
import Testing

private let hubURL = URL(string: "http://localhost:8090")!
private let profileJSON = ##"{"body":"# Candidate","updated_at":"2026-09-28T12:38:11.635355Z"}"##

struct HubClientTests {
    @Test func requestsCarryTheOwnerTokenAndTheJSONBody() throws {
        let client = HubClient(baseURL: hubURL, token: "owner-token")
        let request = client.makeRequest(method: "PUT", path: "v1/profile", body: Data(#"{"body":"x"}"#.utf8))

        #expect(request.url?.absoluteString == "http://localhost:8090/v1/profile")
        #expect(request.httpMethod == "PUT")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer owner-token")
        #expect(request.value(forHTTPHeaderField: "Content-Type") == "application/json")
    }

    @Test func answersMapToErrors() {
        #expect(HubClient.mapFailure(status: 200, body: Data()) == nil)
        #expect(HubClient.mapFailure(status: 401, body: Data()) == .unauthorized)
        #expect(HubClient.mapFailure(status: 403, body: Data()) == .forbidden)
        #expect(HubClient.mapFailure(status: 404, body: Data()) == .notFound)
        #expect(HubClient.mapFailure(status: 500, body: Data(#"{"error":"database down"}"#.utf8)) == .server(status: 500, message: "database down"))
    }

    @Test func goDatesDecodeWithAndWithoutFractionalSeconds() throws {
        let decoder = HubJSON.makeDecoder()
        let withFraction = try decoder.decode(OwnerProfile.self, from: Data(profileJSON.utf8))
        let withoutFraction = try decoder.decode(OwnerProfile.self, from: Data(#"{"body":"","updated_at":"2026-09-28T12:38:11Z"}"#.utf8))

        #expect(withFraction.body == "# Candidate")
        #expect(abs(withFraction.updatedAt.timeIntervalSince(withoutFraction.updatedAt) - 0.635355) < 0.001)
    }

    @Test func aStoppedServerIsNotReportedAsABadToken() async {
        let (session, _) = StubHub.makeSession(answers: [:], isDown: true)
        let status = await ConnectionStatus.check(baseURL: hubURL, token: "owner-token", session: session)
        guard case .serverUnreachable = status else {
            Issue.record("status = \(status), want serverUnreachable")
            return
        }
    }

    @Test func aRefusedTokenIsReportedAsSuch() async {
        let (session, _) = StubHub.makeSession(answers: [
            "/v1/health": .init(status: 200, body: #"{"database":"ok"}"#),
            "/v1/profile": .init(status: 401, body: ""),
        ])
        #expect(await ConnectionStatus.check(baseURL: hubURL, token: "wrong", session: session) == .tokenRefused)
    }

    @Test func aWorkingTokenConnects() async {
        let (session, recording) = StubHub.makeSession(answers: [
            "/v1/health": .init(status: 200, body: #"{"database":"ok"}"#),
            "/v1/profile": .init(status: 200, body: profileJSON),
        ])
        #expect(await ConnectionStatus.check(baseURL: hubURL, token: "owner-token", session: session) == .connected)
        #expect(recording.lastRequest?.value(forHTTPHeaderField: "Authorization") == "Bearer owner-token")
    }

    @Test func noSavedTokenIsSaidSo() async {
        let (session, _) = StubHub.makeSession(answers: ["/v1/health": .init(status: 200, body: "{}")])
        #expect(await ConnectionStatus.check(baseURL: hubURL, token: nil, session: session) == .missingToken)
    }
}

@Test func aPairingLinkCarriesTheAddressAndTokenBothWays() throws {
    let link = try #require(PairingLink.make(hubURL: "https://mac.tailnet.ts.net", token: "hubdev_abc+/="))
    #expect(link.hasPrefix("jobsearchhub://pair?"))
    let parsed = try #require(PairingLink.parse(link))
    #expect(parsed.hubURL == "https://mac.tailnet.ts.net")
    #expect(parsed.token == "hubdev_abc+/=")
    #expect(PairingLink.parse("https://example.com/?token=x") == nil)
}
