import Foundation

struct Artist: Codable, Identifiable, Sendable {
    let id: Int
    let slug: String
    let name: String
    let state: String?
    let city: String?
    let bandcampEmbedURL: String?
    let social: ArtistSocials
    let createdAt: String
    let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id, slug, name, state, city, social
        case bandcampEmbedURL = "bandcamp_embed_url"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct ArtistSocials: Codable, Sendable {
    let instagram: String?
    let facebook: String?
    let twitter: String?
    let youtube: String?
    let spotify: String?
    let soundcloud: String?
    let bandcamp: String?
    let website: String?
}

struct ArtistShowsResponse: Codable, Sendable {
    let shows: [Show]
}
