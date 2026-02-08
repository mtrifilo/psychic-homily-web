import SwiftUI

struct SavedShowsView: View {
    @State private var viewModel = SavedShowsViewModel()

    var body: some View {
        Group {
            if viewModel.isLoading && viewModel.shows.isEmpty {
                LoadingView(message: "Loading saved shows...")
            } else if let error = viewModel.errorMessage, viewModel.shows.isEmpty {
                ErrorView(message: error) {
                    await viewModel.loadSavedShows()
                }
            } else if viewModel.shows.isEmpty {
                VStack(spacing: 16) {
                    Image(systemName: "bookmark")
                        .font(.system(size: 48))
                        .foregroundStyle(.secondary)

                    Text("No saved shows yet")
                        .font(.title3)
                        .fontWeight(.medium)

                    Text("Tap the bookmark icon on any show to save it here.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 32)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    ForEach(viewModel.shows) { show in
                        NavigationLink(value: show) {
                            ShowRowView(show: show)
                        }
                        .swipeActions(edge: .trailing) {
                            Button(role: .destructive) {
                                Task { await viewModel.unsaveShow(show.id) }
                            } label: {
                                Label("Remove", systemImage: "bookmark.slash")
                            }
                        }
                    }
                }
                .listStyle(.plain)
                .refreshable {
                    await viewModel.loadSavedShows()
                }
            }
        }
        .navigationTitle("My List")
        .navigationDestination(for: Show.self) { show in
            ShowDetailView(idOrSlug: show.slug)
        }
        .task {
            if viewModel.shows.isEmpty {
                await viewModel.loadSavedShows()
            }
        }
    }
}

#Preview {
    NavigationStack {
        SavedShowsView()
    }
    .environment(AppState())
    .preferredColorScheme(.dark)
}
