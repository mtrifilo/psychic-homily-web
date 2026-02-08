import SwiftUI

@Observable
final class ArtistDetailViewModel {
    var artist: Artist?
    var shows: [Show] = []
    var isLoading = false
    var errorMessage: String?

    func loadArtist(idOrSlug: String) async {
        isLoading = true
        errorMessage = nil

        do {
            async let artistResult: Artist = APIClient.shared.request(.getArtist(idOrSlug: idOrSlug))
            async let showsResult: ArtistShowsResponse = APIClient.shared.request(.getArtistShows(idOrSlug: idOrSlug))

            artist = try await artistResult
            shows = try await showsResult.shows
        } catch {
            errorMessage = error.localizedDescription
        }

        isLoading = false
    }
}
