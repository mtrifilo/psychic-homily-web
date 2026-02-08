import SwiftUI

@Observable
final class ShowListViewModel {
    var shows: [Show] = []
    var cities: [ShowCity] = []
    var selectedCity: ShowCity?
    var isLoading = false
    var isLoadingMore = false
    var errorMessage: String?
    var hasMore = true

    private var nextCursor: String?
    private let pageSize = 50
    private let timezone = "America/Phoenix"

    func loadShows() async {
        isLoading = true
        errorMessage = nil
        nextCursor = nil
        hasMore = true

        do {
            let response: UpcomingShowsResponse = try await APIClient.shared.request(
                .upcomingShows(
                    cursor: nil,
                    limit: pageSize,
                    city: selectedCity?.city,
                    timezone: timezone
                )
            )
            shows = response.shows
            nextCursor = response.pagination.nextCursor
            hasMore = response.pagination.hasMore
        } catch {
            errorMessage = error.localizedDescription
        }

        isLoading = false
    }

    func loadMoreIfNeeded(currentShow: Show) async {
        guard hasMore,
              !isLoadingMore,
              let lastShow = shows.last,
              currentShow.id == lastShow.id else {
            return
        }

        isLoadingMore = true

        do {
            let response: UpcomingShowsResponse = try await APIClient.shared.request(
                .upcomingShows(
                    cursor: nextCursor,
                    limit: pageSize,
                    city: selectedCity?.city,
                    timezone: timezone
                )
            )
            shows.append(contentsOf: response.shows)
            nextCursor = response.pagination.nextCursor
            hasMore = response.pagination.hasMore
        } catch {
            // Silently fail on pagination errors — user can scroll again to retry
        }

        isLoadingMore = false
    }

    func loadCities() async {
        do {
            let response: ShowCitiesResponse = try await APIClient.shared.request(
                .showCities(timezone: timezone)
            )
            cities = response.cities
        } catch {
            // Cities are non-critical, silently fail
        }
    }

    func selectCity(_ city: ShowCity?) async {
        selectedCity = city
        await loadShows()
    }

    func refresh() async {
        await loadShows()
    }
}
