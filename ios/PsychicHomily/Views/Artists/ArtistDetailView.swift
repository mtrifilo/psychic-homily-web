import SwiftUI

struct ArtistDetailView: View {
    let idOrSlug: String

    @State private var viewModel = ArtistDetailViewModel()

    var body: some View {
        Group {
            if viewModel.isLoading {
                LoadingView(message: "Loading artist...")
            } else if let error = viewModel.errorMessage {
                ErrorView(message: error) {
                    await viewModel.loadArtist(idOrSlug: idOrSlug)
                }
            } else if let artist = viewModel.artist {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Header
                        VStack(alignment: .leading, spacing: 8) {
                            Text(artist.name)
                                .font(.largeTitle)
                                .fontWeight(.bold)

                            if let city = artist.city, let state = artist.state {
                                Label("\(city), \(state)", systemImage: "mappin")
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .padding(.horizontal)

                        // Social links
                        SocialLinksView(artistSocials: artist.social)

                        Divider().padding(.horizontal)

                        // Upcoming shows
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Upcoming Shows")
                                .font(.headline)
                                .padding(.horizontal)

                            if viewModel.shows.isEmpty {
                                Text("No upcoming shows")
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                                    .padding(.horizontal)
                            } else {
                                ForEach(viewModel.shows) { show in
                                    NavigationLink(value: show) {
                                        ShowRowView(show: show)
                                            .padding(.horizontal)
                                    }
                                    .tint(.primary)

                                    if show.id != viewModel.shows.last?.id {
                                        Divider().padding(.horizontal, 32)
                                    }
                                }
                            }
                        }
                    }
                    .padding(.vertical)
                }
                .navigationDestination(for: Show.self) { show in
                    ShowDetailView(idOrSlug: show.slug)
                }
            }
        }
        .navigationTitle(viewModel.artist?.name ?? "Artist")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await viewModel.loadArtist(idOrSlug: idOrSlug)
        }
    }
}

#Preview {
    NavigationStack {
        ArtistDetailView(idOrSlug: "test-artist")
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
