import Foundation

/// Answers URLSession requests from a table of canned responses, keyed by
/// path. Each session gets its own recording, found through a header the
/// session adds to every request, so tests running in parallel never see each
/// other's requests.
public final class StubHub: URLProtocol, @unchecked Sendable {
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
        private var heldKeys: Set<String> = []
        private var heldAnswers: [String: [@Sendable () -> Void]] = [:]
        private var arrivals: [String: [CheckedContinuation<Void, Never>]] = [:]

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

        /// Holds the answer to each request for `key`, a path or a path
        /// and query, until `release(_:)`, as a slow hub would.
        public func hold(_ key: String) {
            lock.withLock { _ = heldKeys.insert(key) }
        }

        /// Returns once a request for the held `key` waits for its answer.
        public func waitForHeld(_ key: String) async {
            await withCheckedContinuation { continuation in
                let hasArrived = lock.withLock {
                    if heldAnswers[key]?.isEmpty == false { return true }
                    arrivals[key, default: []].append(continuation)
                    return false
                }
                if hasArrived { continuation.resume() }
            }
        }

        /// Answers the requests held for `key`, and those after them at once.
        public func release(_ key: String) {
            let answers = lock.withLock {
                heldKeys.remove(key)
                return heldAnswers.removeValue(forKey: key) ?? []
            }
            for answer in answers {
                DispatchQueue.global().async(execute: answer)
            }
        }

        /// Keeps `answer` for later when one of `keys` is held, and says
        /// whether it did.
        func holdAnswer(for keys: [String], _ answer: @escaping @Sendable () -> Void) -> Bool {
            let waiting: [CheckedContinuation<Void, Never>]? = lock.withLock {
                guard let key = keys.first(where: heldKeys.contains) else { return nil }
                heldAnswers[key, default: []].append(answer)
                return arrivals.removeValue(forKey: key) ?? []
            }
            guard let waiting else { return false }
            for continuation in waiting {
                continuation.resume()
            }
            return true
        }

        public var lastRequest: URLRequest? { lock.withLock { requests.last?.request } }
        public var lastBody: Data? { lock.withLock { requests.last?.body } }
        public var requestCount: Int { lock.withLock { requests.count } }
    }

    private static let recordingHeader = "X-Stub-Recording"
    private static let lock = NSLock()
    nonisolated(unsafe) private static var recordings: [String: Recording] = [:]
    private let stopLock = NSLock()
    /// The request was cancelled, so a held answer is no longer given.
    private var isStopped = false

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
        let keys = (request.url?.query().map { [path + "?" + $0] } ?? []) + [path]
        if recording.holdAnswer(for: keys, { self.answer(from: recording, keys: keys) }) {
            return
        }
        answer(from: recording, keys: keys)
    }

    override public func stopLoading() {
        stopLock.withLock { isStopped = true }
    }

    private func answer(from recording: Recording, keys: [String]) {
        guard !stopLock.withLock({ isStopped }) else { return }
        let answer = keys.lazy.compactMap(recording.getAnswer(for:)).first ?? Answer(status: 404, body: #"{"error":"not found"}"#)
        let response = HTTPURLResponse(url: request.url!, statusCode: answer.status, httpVersion: nil, headerFields: nil)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(answer.body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

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
