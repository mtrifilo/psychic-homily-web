import Foundation

struct SavedShowsResponse: Codable, Sendable {
    let shows: [Show]
}

struct SaveShowResponse: Codable, Sendable {
    let success: Bool
    let message: String?
}

struct CheckSavedResponse: Codable, Sendable {
    let saved: Bool
}
