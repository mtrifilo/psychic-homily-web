import SwiftUI

struct ShowDetailView: View {
    let idOrSlug: String

    @State private var viewModel = ShowDetailViewModel()
    @Environment(AppState.self) private var appState

    var body: some View {
        Group {
            if viewModel.isLoading {
                LoadingView(message: "Loading show...")
            } else if let error = viewModel.errorMessage {
                ErrorView(message: error) {
                    await viewModel.loadShow(idOrSlug: idOrSlug)
                }
            } else if let show = viewModel.show {
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Date header
                        VStack(alignment: .leading, spacing: 4) {
                            Text(show.formattedDayOfWeek)
                                .font(.subheadline)
                                .fontWeight(.semibold)
                                .foregroundStyle(.phAccent)

                            Text(show.formattedDate)
                                .font(.title2)
                                .fontWeight(.bold)

                            if !show.formattedTime.isEmpty {
                                Label(show.formattedTime, systemImage: "clock")
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .padding(.horizontal)

                        // Status badges
                        HStack(spacing: 8) {
                            if show.isSoldOut == true { StatusBadge.soldOut() }
                            if show.isCancelled == true { StatusBadge.cancelled() }
                            if show.price == 0 { StatusBadge.free() }
                        }
                        .padding(.horizontal)

                        // Artists section
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Artists")
                                .font(.headline)
                                .padding(.horizontal)

                            ForEach(show.headliners) { artist in
                                NavigationLink(value: artist) {
                                    HStack {
                                        VStack(alignment: .leading) {
                                            Text(artist.name)
                                                .font(.body)
                                                .fontWeight(.semibold)
                                            Text("Headliner")
                                                .font(.caption)
                                                .foregroundStyle(.phAccent)
                                        }
                                        Spacer()
                                        Image(systemName: "chevron.right")
                                            .font(.caption)
                                            .foregroundStyle(.tertiary)
                                    }
                                    .padding(.horizontal)
                                    .padding(.vertical, 8)
                                }
                                .tint(.primary)
                            }

                            ForEach(show.openers) { artist in
                                NavigationLink(value: artist) {
                                    HStack {
                                        Text(artist.name)
                                            .font(.body)
                                        Spacer()
                                        Image(systemName: "chevron.right")
                                            .font(.caption)
                                            .foregroundStyle(.tertiary)
                                    }
                                    .padding(.horizontal)
                                    .padding(.vertical, 6)
                                }
                                .tint(.primary)
                            }
                        }

                        Divider().padding(.horizontal)

                        // Venue section
                        if let venue = show.venues.first {
                            VStack(alignment: .leading, spacing: 8) {
                                Text("Venue")
                                    .font(.headline)

                                NavigationLink(value: venue) {
                                    HStack {
                                        VStack(alignment: .leading, spacing: 4) {
                                            HStack(spacing: 6) {
                                                Text(venue.name)
                                                    .font(.body)
                                                    .fontWeight(.medium)
                                                if venue.verified {
                                                    Image(systemName: "checkmark.seal.fill")
                                                        .font(.caption)
                                                        .foregroundStyle(.blue)
                                                }
                                            }
                                            Text("\(venue.city), \(venue.state)")
                                                .font(.caption)
                                                .foregroundStyle(.secondary)
                                            if let address = venue.address {
                                                Text(address)
                                                    .font(.caption)
                                                    .foregroundStyle(.secondary)
                                            }
                                        }
                                        Spacer()
                                        Image(systemName: "chevron.right")
                                            .font(.caption)
                                            .foregroundStyle(.tertiary)
                                    }
                                }
                                .tint(.primary)
                            }
                            .padding(.horizontal)
                        }

                        Divider().padding(.horizontal)

                        // Details
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Details")
                                .font(.headline)

                            if let priceText = show.priceText {
                                Label(priceText, systemImage: "dollarsign.circle")
                                    .font(.subheadline)
                            }

                            if let age = show.ageRequirement {
                                Label(age, systemImage: "person.badge.shield.checkmark")
                                    .font(.subheadline)
                            }

                            Label("\(show.city), \(show.state)", systemImage: "mappin.and.ellipse")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)

                            if let description = show.description, !description.isEmpty {
                                Text(description)
                                    .font(.body)
                                    .foregroundStyle(.secondary)
                                    .padding(.top, 4)
                            }
                        }
                        .padding(.horizontal)
                    }
                    .padding(.vertical)
                }
                .navigationDestination(for: ShowArtist.self) { artist in
                    ArtistDetailView(idOrSlug: artist.slug)
                }
                .navigationDestination(for: ShowVenue.self) { venue in
                    VenueDetailView(idOrSlug: venue.slug)
                }
            }
        }
        .navigationTitle(viewModel.show?.title ?? "Show")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if appState.isAuthenticated, let show = viewModel.show {
                ToolbarItem(placement: .topBarTrailing) {
                    SaveButton(
                        isSaved: viewModel.isSaved,
                        isSaving: viewModel.isSaving
                    ) {
                        Task { await viewModel.toggleSave(showId: show.id) }
                    }
                }
            }
        }
        .task {
            await viewModel.loadShow(idOrSlug: idOrSlug)
            if let show = viewModel.show {
                await viewModel.checkSaved(showId: show.id)
            }
        }
    }
}

#Preview {
    NavigationStack {
        ShowDetailView(idOrSlug: "test-show")
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
