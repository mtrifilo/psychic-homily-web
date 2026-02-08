import SwiftUI

struct VenueDetailView: View {
    let idOrSlug: String

    @State private var viewModel = VenueDetailViewModel()

    var body: some View {
        Group {
            if viewModel.isLoading {
                LoadingView(message: "Loading venue...")
            } else if let error = viewModel.errorMessage {
                ErrorView(message: error) {
                    await viewModel.loadVenue(idOrSlug: idOrSlug)
                }
            } else if let venue = viewModel.venue {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Header
                        VStack(alignment: .leading, spacing: 8) {
                            HStack(spacing: 8) {
                                Text(venue.name)
                                    .font(.largeTitle)
                                    .fontWeight(.bold)

                                if venue.verified {
                                    Image(systemName: "checkmark.seal.fill")
                                        .foregroundStyle(.blue)
                                }
                            }

                            Label("\(venue.city), \(venue.state)", systemImage: "mappin")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)

                            if let address = venue.address {
                                Text(address)
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .padding(.horizontal)

                        // Social links
                        SocialLinksView(venueSocials: venue.social)

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
        .navigationTitle(viewModel.venue?.name ?? "Venue")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await viewModel.loadVenue(idOrSlug: idOrSlug)
        }
    }
}

#Preview {
    NavigationStack {
        VenueDetailView(idOrSlug: "test-venue")
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
