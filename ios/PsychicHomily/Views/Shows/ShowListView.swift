import SwiftUI

struct ShowListView: View {
    @State private var viewModel = ShowListViewModel()
    @Environment(AppState.self) private var appState

    var body: some View {
        Group {
            if viewModel.isLoading && viewModel.shows.isEmpty {
                LoadingView(message: "Loading shows...")
            } else if let error = viewModel.errorMessage, viewModel.shows.isEmpty {
                ErrorView(message: error) {
                    await viewModel.refresh()
                }
            } else {
                VStack(spacing: 0) {
                    if !viewModel.cities.isEmpty {
                        CityFilterView(
                            cities: viewModel.cities,
                            selectedCity: viewModel.selectedCity
                        ) { city in
                            Task { await viewModel.selectCity(city) }
                        }
                    }

                    List {
                        ForEach(viewModel.shows) { show in
                            NavigationLink(value: show) {
                                ShowRowView(show: show)
                            }
                            .swipeActions(edge: .trailing) {
                                if appState.isAuthenticated {
                                    Button {
                                        Task { await saveShow(show.id) }
                                    } label: {
                                        Label("Save", systemImage: "bookmark")
                                    }
                                    .tint(.phAccent)
                                }
                            }
                            .onAppear {
                                Task { await viewModel.loadMoreIfNeeded(currentShow: show) }
                            }
                        }

                        if viewModel.isLoadingMore {
                            HStack {
                                Spacer()
                                ProgressView()
                                    .padding()
                                Spacer()
                            }
                            .listRowSeparator(.hidden)
                        }
                    }
                    .listStyle(.plain)
                    .refreshable {
                        await viewModel.refresh()
                    }
                }
            }
        }
        .navigationTitle("Shows")
        .navigationDestination(for: Show.self) { show in
            ShowDetailView(idOrSlug: show.slug)
        }
        .task {
            if viewModel.shows.isEmpty {
                async let shows: () = viewModel.loadShows()
                async let cities: () = viewModel.loadCities()
                _ = await (shows, cities)
            }
        }
    }

    private func saveShow(_ showId: Int) async {
        do {
            let _: SaveShowResponse = try await APIClient.shared.request(.saveShow(showId: showId))
        } catch {
            // Non-critical — user can save from detail view
        }
    }
}

#Preview {
    NavigationStack {
        ShowListView()
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
