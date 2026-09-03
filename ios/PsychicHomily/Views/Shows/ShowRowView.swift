import SwiftUI

struct ShowRowView: View {
    let show: Show

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Date and status badges
            HStack {
                Text(show.formattedDayOfWeek)
                    .font(.caption)
                    .fontWeight(.semibold)
                    .foregroundStyle(.phAccent)

                Text(show.formattedDate)
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Spacer()

                if show.isSoldOut == true {
                    StatusBadge.soldOut()
                } else if show.isCancelled == true {
                    StatusBadge.cancelled()
                } else if show.isFree {
                    StatusBadge.free()
                }
            }

            // Artists
            VStack(alignment: .leading, spacing: 2) {
                ForEach(show.headliners) { artist in
                    Text(artist.name)
                        .font(.headline)
                        .lineLimit(1)
                }
                if !show.openers.isEmpty {
                    Text(show.openers.map(\.name).joined(separator: ", "))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }

            // Venue and details
            HStack {
                if let venue = show.venues.first {
                    Label(venue.name, systemImage: "mappin")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }

                Spacer()

                // Suppressed for a free show, whose only price the badge row
                // above already carries. A show priced 0 at advance and $25 at
                // the door is not free and still prints both numbers.
                if let priceText = show.priceText, !show.isFree {
                    Text(priceText)
                        .font(.caption)
                        .fontWeight(.medium)
                        .foregroundStyle(.phSecondary)
                        .accessibilityLabel(show.priceAccessibilityLabel ?? priceText)
                }

                if let time = show.formattedTime.isEmpty ? nil : show.formattedTime {
                    Text(time)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
    }
}
