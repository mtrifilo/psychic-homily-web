import SwiftUI

struct ShowFormView: View {
    let extractedData: ExtractedShowData
    var onSubmitted: (() -> Void)?

    @State private var eventDate = ""
    @State private var eventTime = ""
    @State private var cost = ""
    @State private var ageRequirement = ""
    @State private var description = ""
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    @State private var submissionSuccess = false

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        if submissionSuccess {
            VStack(spacing: 20) {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 64))
                    .foregroundStyle(.green)

                Text("Show Submitted!")
                    .font(.title2)
                    .fontWeight(.bold)

                Text("Your show has been submitted for review.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)

                Button("Done") {
                    onSubmitted?()
                    dismiss()
                }
                .buttonStyle(.borderedProminent)
                .tint(.phPrimary)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            Form {
                // Artists (read-only display)
                Section("Artists") {
                    ForEach(extractedData.artists) { artist in
                        HStack {
                            Text(artist.name)
                            Spacer()
                            if artist.isHeadliner {
                                Text("Headliner")
                                    .font(.caption)
                                    .foregroundStyle(.phAccent)
                            }
                            MatchIndicator(status: artist.matchStatus, matchedName: nil)
                        }
                    }
                }

                // Venue (read-only display)
                if let venue = extractedData.venue {
                    Section("Venue") {
                        HStack {
                            VStack(alignment: .leading) {
                                Text(venue.name)
                                if let city = venue.city, let state = venue.state {
                                    Text("\(city), \(state)")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            Spacer()
                            MatchIndicator(status: venue.matchStatus, matchedName: nil)
                        }
                    }
                }

                // Editable fields
                Section("Event Details") {
                    LabeledContent("Date") {
                        TextField("e.g. 2025-03-15", text: $eventDate)
                            .multilineTextAlignment(.trailing)
                    }

                    LabeledContent("Time") {
                        TextField("e.g. 8:00 PM", text: $eventTime)
                            .multilineTextAlignment(.trailing)
                    }

                    LabeledContent("Price") {
                        TextField("e.g. $15", text: $cost)
                            .multilineTextAlignment(.trailing)
                    }

                    LabeledContent("Ages") {
                        TextField("e.g. 21+", text: $ageRequirement)
                            .multilineTextAlignment(.trailing)
                    }
                }

                Section("Description") {
                    TextEditor(text: $description)
                        .frame(minHeight: 80)
                }

                if let error = errorMessage {
                    Section {
                        Text(error)
                            .foregroundStyle(.red)
                            .font(.caption)
                    }
                }

                Section {
                    Button {
                        Task { await submitShow() }
                    } label: {
                        if isSubmitting {
                            HStack {
                                ProgressView()
                                    .tint(.white)
                                Text("Submitting...")
                            }
                            .frame(maxWidth: .infinity)
                        } else {
                            Text("Submit Show")
                                .fontWeight(.semibold)
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.phPrimary)
                    .disabled(isSubmitting)
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                }
            }
            .navigationTitle("Review Show")
            .navigationBarTitleDisplayMode(.inline)
            .onAppear {
                eventDate = extractedData.date ?? ""
                eventTime = extractedData.time ?? ""
                cost = extractedData.cost ?? ""
                ageRequirement = extractedData.ages ?? ""
                description = extractedData.description ?? ""
            }
        }
    }

    private func submitShow() async {
        isSubmitting = true
        errorMessage = nil

        // Build the artists array for submission
        var artistSubmissions: [[String: Any]] = []
        for artist in extractedData.artists {
            var entry: [String: Any] = ["is_headliner": artist.isHeadliner]
            if let matchedId = artist.matchedId {
                entry["id"] = matchedId
            } else {
                entry["name"] = artist.name
            }
            artistSubmissions.append(entry)
        }

        // Build the venues array
        var venueSubmissions: [[String: Any]] = []
        if let venue = extractedData.venue {
            if let matchedId = venue.matchedId {
                venueSubmissions.append(["id": matchedId])
            } else {
                var entry: [String: Any] = ["name": venue.name]
                if let city = venue.city { entry["city"] = city }
                if let state = venue.state { entry["state"] = state }
                venueSubmissions.append(entry)
            }
        }

        // Parse price from cost string
        let price = parsePrice(cost)

        // Build request body
        var body: [String: Any] = [
            "event_date": eventDate,
            "city": extractedData.venue?.city ?? "Phoenix",
            "state": extractedData.venue?.state ?? "AZ",
            "artists": artistSubmissions,
            "venues": venueSubmissions,
        ]
        if price > 0 { body["price"] = price }
        if !ageRequirement.isEmpty { body["age_requirement"] = ageRequirement }
        if !description.isEmpty { body["description"] = description }

        do {
            let jsonData = try JSONSerialization.data(withJSONObject: body)
            let _: CreateShowResponse = try await APIClient.shared.request(
                .createShow(body: jsonData)
            )
            submissionSuccess = true
        } catch {
            errorMessage = error.localizedDescription
        }

        isSubmitting = false
    }

    private func parsePrice(_ text: String) -> Double {
        let cleaned = text.replacingOccurrences(of: "$", with: "")
            .replacingOccurrences(of: ",", with: "")
            .trimmingCharacters(in: .whitespaces)
        return Double(cleaned) ?? 0
    }
}

struct CreateShowResponse: Codable {
    let success: Bool
    let message: String?
}
