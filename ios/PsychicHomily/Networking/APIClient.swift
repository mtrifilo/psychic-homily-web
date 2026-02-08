import Foundation

enum APIError: Error, LocalizedError {
    case invalidURL
    case noData
    case decodingError(Error)
    case httpError(statusCode: Int, body: APIErrorResponse?)
    case networkError(Error)
    case unauthorized
    case rateLimited(retryAfter: Int?)

    var errorDescription: String? {
        switch self {
        case .invalidURL: return "Invalid URL"
        case .noData: return "No data received"
        case .decodingError(let error): return "Failed to parse response: \(error.localizedDescription)"
        case .httpError(let code, let body):
            return body?.message ?? "Server error (HTTP \(code))"
        case .networkError(let error): return error.localizedDescription
        case .unauthorized: return "Please sign in to continue"
        case .rateLimited: return "Too many requests. Please try again in a moment."
        }
    }
}

actor APIClient {
    static let shared = APIClient()

    #if DEBUG
    private let baseURL = "http://localhost:8080"
    #else
    private let baseURL = "https://api.psychichomily.com"
    #endif

    private let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        return decoder
    }()

    private let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        return encoder
    }()

    private let session: URLSession = {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 30
        config.timeoutIntervalForResource = 60
        return URLSession(configuration: config)
    }()

    // MARK: - Public Methods

    func request<T: Decodable>(_ endpoint: APIEndpoint) async throws -> T {
        let urlRequest = try buildRequest(endpoint)
        return try await execute(urlRequest)
    }

    func request<T: Decodable>(_ endpoint: APIEndpoint, body: some Encodable) async throws -> T {
        var urlRequest = try buildRequest(endpoint)
        urlRequest.httpBody = try encoder.encode(body)
        urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        return try await execute(urlRequest)
    }

    func requestVoid(_ endpoint: APIEndpoint) async throws {
        let urlRequest = try buildRequest(endpoint)
        let (_, response) = try await session.data(for: urlRequest)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.noData
        }

        if httpResponse.statusCode == 429 {
            let retryAfter = httpResponse.value(forHTTPHeaderField: "Retry-After").flatMap(Int.init)
            throw APIError.rateLimited(retryAfter: retryAfter)
        }

        guard (200...299).contains(httpResponse.statusCode) else {
            throw APIError.httpError(statusCode: httpResponse.statusCode, body: nil)
        }
    }

    // MARK: - Private Methods

    private func buildRequest(_ endpoint: APIEndpoint) throws -> URLRequest {
        var components = URLComponents(string: baseURL + endpoint.path)
        components?.queryItems = endpoint.queryItems

        guard let url = components?.url else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = endpoint.method.rawValue

        // Add auth header if needed
        if endpoint.requiresAuth, let token = KeychainManager.getToken() {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        // Add body if the endpoint has one
        if let rawBody = endpoint.rawBody {
            request.httpBody = rawBody
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        } else if let body = endpoint.body {
            request.httpBody = try encoder.encode(body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }

        return request
    }

    private func execute<T: Decodable>(_ request: URLRequest) async throws -> T {
        let data: Data
        let response: URLResponse

        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw APIError.networkError(error)
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.noData
        }

        // Handle rate limiting
        if httpResponse.statusCode == 429 {
            let retryAfter = httpResponse.value(forHTTPHeaderField: "Retry-After").flatMap(Int.init)
            throw APIError.rateLimited(retryAfter: retryAfter)
        }

        // Handle auth errors
        if httpResponse.statusCode == 401 {
            // Try to decode error response for error_code
            if let errorBody = try? decoder.decode(APIErrorResponse.self, from: data),
               errorBody.errorCode == "TOKEN_EXPIRED" {
                // Attempt refresh
                if try await refreshToken() {
                    // Retry original request with new token
                    var retryRequest = request
                    if let token = KeychainManager.getToken() {
                        retryRequest.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                    }
                    return try await executeWithoutRetry(retryRequest)
                }
            }
            throw APIError.unauthorized
        }

        guard (200...299).contains(httpResponse.statusCode) else {
            let errorBody = try? decoder.decode(APIErrorResponse.self, from: data)
            throw APIError.httpError(statusCode: httpResponse.statusCode, body: errorBody)
        }

        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw APIError.decodingError(error)
        }
    }

    private func executeWithoutRetry<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.noData
        }

        if httpResponse.statusCode == 401 {
            throw APIError.unauthorized
        }

        guard (200...299).contains(httpResponse.statusCode) else {
            let errorBody = try? decoder.decode(APIErrorResponse.self, from: data)
            throw APIError.httpError(statusCode: httpResponse.statusCode, body: errorBody)
        }

        return try decoder.decode(T.self, from: data)
    }

    /// Attempt to refresh the token. Returns true if successful.
    private func refreshToken() async throws -> Bool {
        guard KeychainManager.getToken() != nil else { return false }

        let request = try buildRequest(.refreshToken)

        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse,
              httpResponse.statusCode == 200 else {
            // Refresh failed — clear token
            KeychainManager.deleteToken()
            return false
        }

        guard let refreshResponse = try? decoder.decode(RefreshTokenResponse.self, from: data),
              refreshResponse.success,
              let newToken = refreshResponse.token else {
            KeychainManager.deleteToken()
            return false
        }

        KeychainManager.saveToken(newToken)
        return true
    }
}
