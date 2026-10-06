import Foundation

/// Answers URLSession requests from a table of canned responses, keyed by
/// path. Each session gets its own recording, found through a header the
/// session adds to every request, so tests running in parallel never see each
/// other's requests.
public final class StubHub: URLProtocol {
    public struct Answer: Sendable {
        public let status: Int
        public let body: String

        public init(status: Int, body: String) {
            self.status = status
            self.body = body
        }
    }

    public final class Recording: @unchecked Sendable {
        public let isDown: Bool
        private let lock = NSLock()
        private var answers: [String: Answer]
        private var requests: [(request: URLRequest, body: Data)] = []

        init(answers: [String: Answer], isDown: Bool) {
            self.answers = answers
            self.isDown = isDown
        }

        func record(_ request: URLRequest, body: Data) {
            lock.withLock { requests.append((request, body)) }
        }

        func getAnswer(for key: String) -> Answer? {
            lock.withLock { answers[key] }
        }

        /// Answers the path differently from the next request on.
        public func setAnswer(_ answer: Answer, for path: String) {
            lock.withLock { answers[path] = answer }
        }

        public var lastRequest: URLRequest? { lock.withLock { requests.last?.request } }
        public var lastBody: Data? { lock.withLock { requests.last?.body } }
        public var requestCount: Int { lock.withLock { requests.count } }
    }

    private static let recordingHeader = "X-Stub-Recording"
    private static let lock = NSLock()
    nonisolated(unsafe) private static var recordings: [String: Recording] = [:]

    public static func makeSession(answers: [String: Answer], isDown: Bool = false) -> (URLSession, Recording) {
        let recording = Recording(answers: answers, isDown: isDown)
        let recordingID = UUID().uuidString
        lock.withLock { recordings[recordingID] = recording }

        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubHub.self]
        configuration.httpAdditionalHeaders = [recordingHeader: recordingID]
        return (URLSession(configuration: configuration), recording)
    }

    override public class func canInit(with request: URLRequest) -> Bool { true }
    override public class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override public func startLoading() {
        let recordingID = request.value(forHTTPHeaderField: Self.recordingHeader) ?? ""
        guard let recording = Self.lock.withLock({ Self.recordings[recordingID] }) else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        recording.record(request, body: Self.readBody(of: request))
        if recording.isDown {
            client?.urlProtocol(self, didFailWithError: URLError(.cannotConnectToHost))
            return
        }
        // An answer keyed by path and query wins over one keyed by the path alone.
        let path = request.url?.path() ?? ""
        let pathAndQuery = request.url?.query().map { path + "?" + $0 }
        let answer = pathAndQuery.flatMap(recording.getAnswer(for:)) ?? recording.getAnswer(for: path) ?? Answer(status: 404, body: #"{"error":"not found"}"#)
        let response = HTTPURLResponse(url: request.url!, statusCode: answer.status, httpVersion: nil, headerFields: nil)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(answer.body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override public func stopLoading() {}

    /// URLSession hands a protocol the body as a stream, not as httpBody.
    private static func readBody(of request: URLRequest) -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var body = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count <= 0 { break }
            body.append(buffer, count: count)
        }
        return body
    }
}
