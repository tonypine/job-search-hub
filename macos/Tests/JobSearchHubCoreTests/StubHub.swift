import Foundation

/// Answers URLSession requests from a table of canned responses, keyed by
/// path, and remembers each request it saw.
final class StubHub: URLProtocol {
    struct Answer {
        let status: Int
        let body: String
    }

    nonisolated(unsafe) static var answers: [String: Answer] = [:]
    nonisolated(unsafe) static var seenRequests: [URLRequest] = []
    nonisolated(unsafe) static var isDown = false

    static func makeSession(answers: [String: Answer], isDown: Bool = false) -> URLSession {
        Self.answers = answers
        Self.seenRequests = []
        Self.isDown = isDown
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubHub.self]
        return URLSession(configuration: configuration)
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.seenRequests.append(request)
        if Self.isDown {
            client?.urlProtocol(self, didFailWithError: URLError(.cannotConnectToHost))
            return
        }
        let answer = Self.answers[request.url?.path() ?? ""] ?? Answer(status: 404, body: #"{"error":"not found"}"#)
        let response = HTTPURLResponse(url: request.url!, statusCode: answer.status, httpVersion: nil, headerFields: nil)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(answer.body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
