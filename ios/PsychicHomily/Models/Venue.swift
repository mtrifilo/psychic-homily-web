import Foundation

struct Venue: Codable, Identifiable, Sendable {
    let id: Int
    let slug: String
    let name: String
    let address: String?
    let city: String
    let state: String
    let zipcode: String?
    let verified: Bool
    let social: VenueSocials
    let createdAt: String
    let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id, slug, name, address, city, state, zipcode, verified, social
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct VenueSocials: Codable, Sendable {
    let instagram: String?
    let facebook: String?
    let twitter: String?
    let youtube: String?
    let spotify: String?
    let soundcloud: String?
    let bandcamp: String?
    let website: String?
}

struct VenueShowsResponse: Codable, Sendable {
    let shows: [Show]
}
