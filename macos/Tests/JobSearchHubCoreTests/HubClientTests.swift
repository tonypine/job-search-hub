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

    @Test func requestsNameTheAppAndItsVersion() {
        let client = HubClient(baseURL: hubURL, token: "owner-token", appVersion: "0.1.252")
        let request = client.makeRequest(method: "GET", path: "v1/jobs", body: nil)

        #expect(request.value(forHTTPHeaderField: "X-Hub-Client") == "macos/0.1.252")
    }

    @Test func aHubThatNoLongerServesTheAppReachesTheErrorViewInItsOwnWords() async {
        let message = "This app (0.1.199) is too old for the hub, which runs 0.1.250. Update the app to 0.1.200 or later."
        let (session, _) = StubHub.makeSession(answers: [
            "/v1/jobs": .init(status: 426, body: #"{"error":"\#(message)"}"#),
        ])
        let client = HubClient(baseURL: hubURL, token: "owner-token", appVersion: "0.1.199", session: session)
        do {
            _ = try await client.get("v1/jobs", as: OwnerProfile.self)
            Issue.record("a 426 didn't throw")
        } catch {
            #expect(error as? HubError == .upgradeRequired(message))
            #expect(ErrorReport(error).advice == message)
        }
    }

    @Test func theConnectionCheckSaysTheAppIsTooOld() async {
        let (session, _) = StubHub.makeSession(answers: [
            "/v1/health": .init(status: 200, body: #"{"database":"ok"}"#),
            "/v1/profile": .init(status: 426, body: #"{"error":"Update the app to 0.1.200 or later."}"#),
        ])
        let status = await ConnectionStatus.check(baseURL: hubURL, token: "owner-token", session: session)
        #expect(status == .upgradeRequired("Update the app to 0.1.200 or later."))
        #expect(status.message == "Update the app to 0.1.200 or later.")
    }

    @Test func answersMapToErrors() {
        #expect(HubClient.mapFailure(status: 200, body: Data()) == nil)
        #expect(HubClient.mapFailure(status: 401, body: Data()) == .unauthorized)
        #expect(HubClient.mapFailure(status: 403, body: Data()) == .forbidden)
        #expect(HubClient.mapFailure(status: 404, body: Data()) == .notFound)
        #expect(HubClient.mapFailure(status: 500, body: Data(#"{"error":"database down"}"#.utf8)) == .server(status: 500, message: "database down"))
        #expect(HubClient.mapFailure(status: 426, body: Data(#"{"error":"Update the app."}"#.utf8)) == .upgradeRequired("Update the app."))
        if case let .upgradeRequired(message) = HubClient.mapFailure(status: 426, body: Data()) {
            #expect(!message.isEmpty)
        } else {
            Issue.record("a 426 without a body isn't upgradeRequired")
        }
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

@Test func aPhonesAppVersionDecodesWhenTheHubKnowsIt() throws {
    let json = #"{"devices":[{"id":"6b0c3f4e-3c7a-4c38-9a59-8f2a3b1c9d10","name":"Sam's phone","created_at":"2026-10-01T09:00:00Z","app_version":"0.1.252"},{"id":"7c1d4f5e-3c7a-4c38-9a59-8f2a3b1c9d10","name":"Old phone","created_at":"2026-10-01T09:00:00Z"}]}"#
    let devices = try HubJSON.makeDecoder().decode(DevicesResponse.self, from: Data(json.utf8)).devices

    #expect(devices.map(\.appVersion) == ["0.1.252", nil])
}

@Test func onlyAnAddressOffTheMacReachesAPhone() {
    #expect(PhoneHubAddress.isUnreachableFromPhone(""))
    #expect(PhoneHubAddress.isUnreachableFromPhone("http://10.0.2.2:8090"))
    #expect(PhoneHubAddress.isUnreachableFromPhone("http://localhost:8090"))
    #expect(PhoneHubAddress.isUnreachableFromPhone(" http://127.0.0.1:8090 "))
    #expect(!PhoneHubAddress.isUnreachableFromPhone("https://mac.tailnet.ts.net"))
    #expect(!PhoneHubAddress.isUnreachableFromPhone("http://192.168.1.20:8090"))
}

@Test func tailscaleStatusGivesTheMacsHTTPSAddressOnceItHasCertificates() {
    let withCertificates = Data(#"{"Self":{"DNSName":"mac.tailnet.ts.net."},"CertDomains":["mac.tailnet.ts.net"]}"#.utf8)
    #expect(PhoneHubAddress.parseTailscaleStatusToAddress(withCertificates) == "https://mac.tailnet.ts.net")
    let withoutCertificates = Data(#"{"Self":{"DNSName":"mac.tailnet.ts.net."},"CertDomains":null}"#.utf8)
    #expect(PhoneHubAddress.parseTailscaleStatusToAddress(withoutCertificates) == nil)
    #expect(PhoneHubAddress.parseTailscaleStatusToAddress(Data("The Tailscale CLI failed to start".utf8)) == nil)
}

@Test func aPairingLinkCarriesTheAddressAndTokenBothWays() throws {
    let link = try #require(PairingLink.make(hubURL: "https://mac.tailnet.ts.net", token: "hubdev_abc+/="))
    #expect(link.hasPrefix("jobsearchhub://pair?"))
    let parsed = try #require(PairingLink.parse(link))
    #expect(parsed.hubURL == "https://mac.tailnet.ts.net")
    #expect(parsed.token == "hubdev_abc+/=")
    #expect(PairingLink.parse("https://example.com/?token=x") == nil)
}

@Test func knowledgeBaseEntriesDecode() throws {
    let json = #"{"entries":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","kind":"case","role_id":"0aa55565-58d2-4247-ba01-cba65060a316","title":"Checkout on one page","body":"Rebuilt it","organization":"","start_month":"2022-05","end_month":"","skills":["React"],"outcome":"Faster","source":"interview","source_detail":"","updated_at":"2026-09-29T12:00:00Z"}]}"#
    let response = try HubJSON.makeDecoder().decode(ProfileEntriesResponse.self, from: Data(json.utf8))
    #expect(response.entries.first?.roleID != nil && response.entries.first?.isConfirmed == false && response.entries.first?.startMonth == "2022-05")
}

@Test func aSeedRunReportsWhatItAdded() {
    let output = "Building the knowledge base…\n\nTwo roles.\n  to settle: dates differ\nEntries added: 12, updated: 3\n"
    #expect(ProfileSeedLaunch.parseOutcome(output)?.added == 12 && ProfileSeedLaunch.parseOutcome(output)?.updated == 3)
    #expect(ProfileSeedLaunch.parseOutcome("the run failed") == nil)
}
