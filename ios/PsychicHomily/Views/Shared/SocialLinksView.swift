import SwiftUI

struct SocialLink: Identifiable {
    let id = UUID()
    let name: String
    let icon: String
    let url: String
}

struct SocialLinksView: View {
    let links: [SocialLink]

    var body: some View {
        if !links.isEmpty {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 12) {
                    ForEach(links) { link in
                        Link(destination: URL(string: link.url)!) {
                            Label(link.name, systemImage: link.icon)
                                .font(.caption)
                                .padding(.horizontal, 12)
                                .padding(.vertical, 8)
                                .background(.phSurface)
                                .clipShape(Capsule())
                        }
                        .tint(.phSecondary)
                    }
                }
                .padding(.horizontal)
            }
        }
    }

    init(artistSocials: ArtistSocials) {
        var result: [SocialLink] = []
        if let url = artistSocials.website { result.append(SocialLink(name: "Website", icon: "globe", url: url)) }
        if let url = artistSocials.instagram { result.append(SocialLink(name: "Instagram", icon: "camera", url: url)) }
        if let url = artistSocials.spotify { result.append(SocialLink(name: "Spotify", icon: "music.note", url: url)) }
        if let url = artistSocials.bandcamp { result.append(SocialLink(name: "Bandcamp", icon: "music.mic", url: url)) }
        if let url = artistSocials.soundcloud { result.append(SocialLink(name: "SoundCloud", icon: "cloud", url: url)) }
        if let url = artistSocials.youtube { result.append(SocialLink(name: "YouTube", icon: "play.rectangle", url: url)) }
        if let url = artistSocials.facebook { result.append(SocialLink(name: "Facebook", icon: "person.2", url: url)) }
        if let url = artistSocials.twitter { result.append(SocialLink(name: "Twitter", icon: "at", url: url)) }
        self.links = result
    }

    init(venueSocials: VenueSocials) {
        var result: [SocialLink] = []
        if let url = venueSocials.website { result.append(SocialLink(name: "Website", icon: "globe", url: url)) }
        if let url = venueSocials.instagram { result.append(SocialLink(name: "Instagram", icon: "camera", url: url)) }
        if let url = venueSocials.spotify { result.append(SocialLink(name: "Spotify", icon: "music.note", url: url)) }
        if let url = venueSocials.bandcamp { result.append(SocialLink(name: "Bandcamp", icon: "music.mic", url: url)) }
        if let url = venueSocials.soundcloud { result.append(SocialLink(name: "SoundCloud", icon: "cloud", url: url)) }
        if let url = venueSocials.youtube { result.append(SocialLink(name: "YouTube", icon: "play.rectangle", url: url)) }
        if let url = venueSocials.facebook { result.append(SocialLink(name: "Facebook", icon: "person.2", url: url)) }
        if let url = venueSocials.twitter { result.append(SocialLink(name: "Twitter", icon: "at", url: url)) }
        self.links = result
    }
}
