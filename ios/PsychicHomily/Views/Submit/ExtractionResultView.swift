import SwiftUI

struct ExtractionResultView: View {
    let data: ExtractedShowData
    let warnings: [String]?
    var onSubmitted: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("Extracted Info")
                .font(.title2)
                .fontWeight(.bold)

            // Warnings
            if let warnings, !warnings.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(warnings, id: \.self) { warning in
                        Label(warning, systemImage: "exclamationmark.triangle")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }
                }
                .padding()
                .background(Color.orange.opacity(0.1))
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }

            // Artists
            VStack(alignment: .leading, spacing: 8) {
                Text("Artists")
                    .font(.headline)

                ForEach(data.artists) { artist in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(artist.name)
                                .font(.body)
                                .fontWeight(artist.isHeadliner ? .semibold : .regular)

                            if artist.isHeadliner {
                                Text("Headliner")
                                    .font(.caption2)
                                    .foregroundStyle(.phAccent)
                            }
                        }

                        Spacer()

                        MatchIndicator(status: artist.matchStatus, matchedName: artist.matchedName)
                    }
                    .padding(.vertical, 4)
                }
            }

            Divider()

            // Venue
            if let venue = data.venue {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Venue")
                        .font(.headline)

                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(venue.name)
                                .font(.body)
                            if let city = venue.city, let state = venue.state {
                                Text("\(city), \(state)")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }

                        Spacer()

                        MatchIndicator(status: venue.matchStatus, matchedName: venue.matchedName)
                    }
                }
            }

            Divider()

            // Event details
            VStack(alignment: .leading, spacing: 8) {
                Text("Details")
                    .font(.headline)

                if let date = data.date {
                    Label(date, systemImage: "calendar")
                        .font(.subheadline)
                }

                if let time = data.time {
                    Label(time, systemImage: "clock")
                        .font(.subheadline)
                }

                if let cost = data.cost {
                    Label(cost, systemImage: "dollarsign.circle")
                        .font(.subheadline)
                }

                if let ages = data.ages {
                    Label(ages, systemImage: "person.badge.shield.checkmark")
                        .font(.subheadline)
                }
            }

            // Navigate to form
            NavigationLink {
                ShowFormView(extractedData: data, onSubmitted: onSubmitted)
            } label: {
                Text("Review & Submit")
                    .fontWeight(.semibold)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 4)
            }
            .buttonStyle(.borderedProminent)
            .tint(.phPrimary)
        }
    }
}

// MARK: - Match Indicator

struct MatchIndicator: View {
    let status: MatchStatus
    let matchedName: String?

    var body: some View {
        switch status {
        case .matched:
            Label(matchedName ?? "Matched", systemImage: "checkmark.circle.fill")
                .font(.caption)
                .foregroundStyle(.green)
        case .suggested:
            Label("Similar found", systemImage: "questionmark.circle")
                .font(.caption)
                .foregroundStyle(.orange)
        case .new:
            Label("New", systemImage: "plus.circle")
                .font(.caption)
                .foregroundStyle(.phSecondary)
        }
    }
}
