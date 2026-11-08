import Foundation

enum RiffleError: LocalizedError, Equatable {
    case invalidAddress
    case unauthorized
    case notFound
    case server(Int, String?)
    case unreachable(String)
    case badResponse

    var errorDescription: String? {
        switch self {
        case .invalidAddress: "That doesn't look like a server address. Try something like tower:8770."
        case .unauthorized: "Riffle Server didn't accept the token. Check it matches RIFFLE_TOKEN."
        case .notFound: "Riffle Server doesn't have this anymore. Try rescanning the library."
        case .server(let code, let msg): "Riffle Server had a problem (\(code)). \(msg ?? "")"
        case .unreachable: "Can't reach Riffle Server. Check that it's running and that you're on the same network or Tailscale."
        case .badResponse: "Riffle Server sent something unexpected. Check that the address points at Riffle Server."
        }
    }
}

/// A resource the server renders on demand: ready, still being made, or never coming.
enum Pending<Value: Sendable>: Sendable {
    case ready(Value)
    case pending(retryAfter: TimeInterval)
    case unavailable
}

/// Talks to Riffle Server. Stateless and `Sendable`: a new one is made when the
/// address or token changes.
struct RiffleClient: Sendable {
    let baseURL: URL
    let token: String
    var session: URLSession = .riffle

    /// Accepts "tower:8770", "192.168.1.10", "http://tower:8770/" and friends.
    /// No port means the server's default, 8770.
    static func normalize(_ address: String) -> URL? {
        var s = address.trimmingCharacters(in: .whitespacesAndNewlines)
        while s.hasSuffix("/") { s.removeLast() }
        guard !s.isEmpty else { return nil }
        if !s.contains("://") { s = "http://" + s }
        guard var c = URLComponents(string: s), let host = c.host, !host.isEmpty,
              c.scheme == "http" || c.scheme == "https" else { return nil }
        if c.port == nil && c.scheme == "http" { c.port = 8770 }
        c.path = ""
        return c.url
    }

    // MARK: Endpoints

    func health() async throws -> HealthInfo {
        try await decode(get("api/health", auth: false))
    }

    enum Fetch<Value: Sendable>: Sendable {
        case fresh(Value, etag: String?)
        case notModified
    }

    /// The library, gzipped by URLSession. Pass the last ETag to get a cheap 304.
    func library(etag: String?) async throws -> Fetch<LibraryResponse> {
        try await cached("api/library", etag: etag)
    }

    func progress(etag: String?) async throws -> Fetch<[String: WatchProgress]> {
        try await cached("api/progress", etag: etag)
    }

    func item(_ id: String) async throws -> Item {
        try await decode(get("api/items/\(id)"))
    }

    func putProgress(_ id: String, position: Double, duration: Double) async throws {
        var req = request("api/items/\(id)/progress", method: "PUT")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONEncoder().encode(["position": position, "duration": duration])
        _ = try await send(req)
    }

    /// Spins up the array disk while the viewer is still on the detail screen.
    func warm(_ id: String) async throws {
        _ = try await send(request("api/items/\(id)/warm", method: "POST"))
    }

    func rescan() async throws {
        _ = try await send(request("api/library/rescan", method: "POST"))
    }

    func poster(_ id: String) async throws -> Pending<Data> {
        try await pending("api/items/\(id)/poster.jpg") { data in data }
    }

    func trickplayManifest(_ id: String) async throws -> Pending<TrickplayManifest> {
        try await pending("api/items/\(id)/trickplay/manifest.json") { data in
            try Self.decoder.decode(TrickplayManifest.self, from: data)
        }
    }

    func trickplaySheet(_ id: String, index: Int, format: String = "jpg") async throws -> Data {
        try await get("api/items/\(id)/trickplay/\(index).\(format)")
    }

    /// The original file. The player sends `streamHeaders` with it rather than putting
    /// the token in the URL, so it never lands in mpv's logs.
    func streamURL(_ id: String) -> URL {
        baseURL.appending(path: "api/items/\(id)/stream")
    }

    var streamHeaders: [String: String] {
        token.isEmpty ? [:] : ["Authorization": "Bearer \(token)"]
    }

    // MARK: Plumbing

    static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { decoder in
            let s = try decoder.singleValueContainer().decode(String.self)
            if let date = parseDate(s) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "bad date \(s)"))
        }
        return d
    }()

    static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    /// Go's `time.Time` is RFC 3339, with or without fractional seconds.
    static func parseDate(_ s: String) -> Date? {
        if let d = try? Date(s, strategy: .iso8601.year().month().day().time(includingFractionalSeconds: true).timeZone(separator: .omitted)) { return d }
        if let d = try? Date(s, strategy: .iso8601) { return d }
        // Nanosecond precision trips the parsers above; trim to milliseconds.
        if let dot = s.firstIndex(of: "."),
           let zone = s[dot...].firstIndex(where: { $0 == "Z" || $0 == "+" || $0 == "-" }) {
            let frac = s[s.index(after: dot)..<zone].prefix(3)
            let trimmed = String(s[..<dot]) + "." + frac + String(s[zone...])
            let f = ISO8601DateFormatter()
            f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            return f.date(from: trimmed)
        }
        return nil
    }

    private func request(_ path: String, method: String = "GET", auth: Bool = true) -> URLRequest {
        var req = URLRequest(url: baseURL.appending(path: path))
        req.httpMethod = method
        req.cachePolicy = .reloadIgnoringLocalCacheData
        if auth && !token.isEmpty {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return req
    }

    private func get(_ path: String, auth: Bool = true) async throws -> Data {
        try await send(request(path, auth: auth)).0
    }

    private func decode<T: Decodable>(_ data: Data) throws -> T {
        do { return try Self.decoder.decode(T.self, from: data) } catch { throw RiffleError.badResponse }
    }

    private func cached<T: Decodable & Sendable>(_ path: String, etag: String?) async throws -> Fetch<T> {
        var req = request(path)
        if let etag { req.setValue(etag, forHTTPHeaderField: "If-None-Match") }
        let (data, resp) = try await send(req, accept: [200, 304])
        if resp.statusCode == 304 { return .notModified }
        return .fresh(try decode(data), etag: resp.value(forHTTPHeaderField: "ETag"))
    }

    private func pending<T: Sendable>(_ path: String, parse: (Data) throws -> T) async throws -> Pending<T> {
        let (data, resp) = try await send(request(path), accept: [200, 202, 404])
        switch resp.statusCode {
        case 200:
            do { return .ready(try parse(data)) } catch { throw RiffleError.badResponse }
        case 202:
            let retry = resp.value(forHTTPHeaderField: "Retry-After").flatMap(TimeInterval.init) ?? 5
            return .pending(retryAfter: retry)
        default:
            return .unavailable
        }
    }

    @discardableResult
    private func send(_ req: URLRequest, accept: Set<Int> = [200, 202, 204]) async throws -> (Data, HTTPURLResponse) {
        let data: Data, response: URLResponse
        do {
            (data, response) = try await session.data(for: req)
        } catch let error as URLError where error.code == .cancelled {
            throw CancellationError()
        } catch {
            throw RiffleError.unreachable(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw RiffleError.badResponse }
        if accept.contains(http.statusCode) { return (data, http) }
        switch http.statusCode {
        case 401, 403: throw RiffleError.unauthorized
        case 404: throw RiffleError.notFound
        default:
            let msg = (try? JSONDecoder().decode([String: String].self, from: data))?["error"]
            throw RiffleError.server(http.statusCode, msg)
        }
    }
}

extension URLSession {
    /// Short timeouts: the server is on the LAN, and a slow answer means it's down.
    /// (Disk spin-up only delays the stream, which goes through mpv, not this session.)
    static let riffle: URLSession = {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.waitsForConnectivity = false
        config.httpMaximumConnectionsPerHost = 6
        config.urlCache = nil
        return URLSession(configuration: config)
    }()
}
