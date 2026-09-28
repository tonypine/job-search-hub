import Foundation

public enum HubError: Error, Equatable, Sendable {
    case unreachable(String)
    case unauthorized
    case forbidden
    case notFound
    case server(status: Int, message: String)
    case undecodable(String)
}

/// Talks to the hub's REST API as the owner.
public struct HubClient: Sendable {
    public let baseURL: URL
    public let token: String
    private let session: URLSession

    public init(baseURL: URL, token: String, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    public func get<Response: Decodable>(
        _ path: String, query: [URLQueryItem] = [], as responseType: Response.Type = Response.self
    ) async throws -> Response {
        try await perform(makeRequest(method: "GET", path: path, query: query, body: nil))
    }

    public func send<Body: Encodable, Response: Decodable>(
        _ method: String, _ path: String, body: Body, as responseType: Response.Type = Response.self
    ) async throws -> Response {
        let encoded: Data
        do {
            encoded = try HubJSON.makeEncoder().encode(body)
        } catch {
            throw HubError.undecodable("encode the request: \(error.localizedDescription)")
        }
        return try await perform(makeRequest(method: method, path: path, body: encoded))
    }

    /// Opens a server-sent event stream and returns its lines as they arrive,
    /// resuming after lastEventID when there is one.
    public func openEventStream(_ path: String, lastEventID: String?) async throws -> EventStreamLines<URLSession.AsyncBytes> {
        var request = makeRequest(method: "GET", path: path, body: nil)
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.timeoutInterval = 90
        if let lastEventID {
            request.setValue(lastEventID, forHTTPHeaderField: "Last-Event-ID")
        }
        let bytes: URLSession.AsyncBytes
        let response: URLResponse
        do {
            (bytes, response) = try await session.bytes(for: request)
        } catch {
            throw HubError.unreachable(error.localizedDescription)
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if let failure = Self.mapFailure(status: status, body: Data()) {
            throw failure
        }
        return EventStreamLines(bytes)
    }

    /// Sends a DELETE, which the hub answers with no body on success.
    public func delete(_ path: String) async throws {
        _ = try await exchange(makeRequest(method: "DELETE", path: path, body: nil))
    }

    func makeRequest(method: String, path: String, query: [URLQueryItem] = [], body: Data?) -> URLRequest {
        var url = baseURL.appending(path: path)
        if !query.isEmpty {
            url.append(queryItems: query)
        }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return request
    }

    private func perform<Response: Decodable>(_ request: URLRequest) async throws -> Response {
        let data = try await exchange(request)
        do {
            return try HubJSON.makeDecoder().decode(Response.self, from: data)
        } catch {
            throw HubError.undecodable(String(describing: error))
        }
    }

    /// Sends the request and returns the body of a 2xx answer; any other
    /// answer throws the error it stands for.
    private func exchange(_ request: URLRequest) async throws -> Data {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw HubError.unreachable(error.localizedDescription)
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if let failure = Self.mapFailure(status: status, body: data) {
            throw failure
        }
        return data
    }

    /// The error a non-2xx answer stands for, or nil for a success.
    static func mapFailure(status: Int, body: Data) -> HubError? {
        switch status {
        case 200..<300: return nil
        case 401: return .unauthorized
        case 403: return .forbidden
        case 404: return .notFound
        default:
            let message = (try? JSONDecoder().decode(ErrorBody.self, from: body))?.error
                ?? String(decoding: body, as: UTF8.self)
            return .server(status: status, message: message)
        }
    }

    private struct ErrorBody: Decodable {
        let error: String
    }
}

/// The JSON conventions of the hub: snake_case keys and RFC 3339 dates, with
/// or without fractional seconds (Go writes them when they are non-zero).
public enum HubJSON {
    public static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            if let date = try? Date(text, strategy: .iso8601.year().month().day().time(includingFractionalSeconds: true).timeZone(separator: .omitted)) {
                return date
            }
            if let date = try? Date(text, strategy: .iso8601) {
                return date
            }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "not an RFC 3339 date: \(text)"))
        }
        return decoder
    }

    public static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        return encoder
    }
}
