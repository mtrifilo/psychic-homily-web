import Testing
import Foundation
@testable import PsychicHomily

@Suite("PsychicHomily Tests")
struct PsychicHomilyTests {
    @Test func appStateDefaultsToShowsTab() {
        let appState = AppState()
        #expect(appState.selectedTab == .shows)
        #expect(appState.isAuthenticated == false)
    }
}

@Suite("Show price register")
struct ShowPriceTests {
    /// A show carrying only the prices under test.
    private func show(price: Double?, doorPrice: Double?) throws -> Show {
        var fields: [String: Any] = [
            "id": 1,
            "slug": "test-show",
            "title": "Test Show",
            "event_date": "2026-04-15T03:00:00Z",
            "city": "Phoenix",
            "state": "AZ",
            "status": "approved",
            "artists": [],
            "venues": [],
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        ]
        if let price { fields["price"] = price }
        if let doorPrice { fields["door_price"] = doorPrice }
        let data = try JSONSerialization.data(withJSONObject: fields)
        return try JSONDecoder().decode(Show.self, from: data)
    }

    @Test func decodesDoorPrice() throws {
        let show = try show(price: 20, doorPrice: 25)
        #expect(show.price == 20)
        #expect(show.doorPrice == 25)
    }

    @Test func decodesAnAbsentDoorPriceAsNil() throws {
        let show = try show(price: 20, doorPrice: nil)
        #expect(show.doorPrice == nil)
        #expect(show.statedPrices == [20])
    }

    @Test func statesBothPricesWhenTheyDiffer() throws {
        #expect(try show(price: 20, doorPrice: 25).statedPrices == [20, 25])
    }

    @Test func collapsesAnEqualPairToOnePrice() throws {
        #expect(try show(price: 20, doorPrice: 20).statedPrices == [20])
    }

    @Test func statesADoorOnlyPriceAsASinglePrice() throws {
        #expect(try show(price: nil, doorPrice: 25).statedPrices == [25])
    }

    @Test func statesNothingWhenNeitherPriceIsRecorded() throws {
        let show = try show(price: nil, doorPrice: nil)
        #expect(show.statedPrices.isEmpty)
        #expect(show.priceText == nil)
        #expect(show.detailPriceText == nil)
        #expect(show.isFree == false)
    }

    @Test func rowSpellsAPairWithASlash() throws {
        #expect(try show(price: 20, doorPrice: 25).priceText == "$20/$25")
    }

    @Test func rowSpellsALonePriceBare() throws {
        #expect(try show(price: 20, doorPrice: nil).priceText == "$20")
        #expect(try show(price: nil, doorPrice: 25).priceText == "$25")
    }

    @Test func detailQualifiesAPairAndLeavesALonePriceBare() throws {
        #expect(try show(price: 20, doorPrice: 25).detailPriceText == "$20 ADV · DOOR $25")
        #expect(try show(price: nil, doorPrice: 25).detailPriceText == "$25")
    }

    @Test func zeroIsAPriceCalledFree() throws {
        let free = try show(price: 0, doorPrice: nil)
        #expect(free.priceText == "Free")
        #expect(free.isFree)
    }

    @Test func aZeroAdvanceWithAPaidDoorIsNotAFreeShow() throws {
        let show = try show(price: 0, doorPrice: 25)
        #expect(show.isFree == false)
        #expect(show.priceText == "Free/$25")
    }

    @Test func spellsFractionalAmountsToTheCent() throws {
        #expect(try show(price: 20.5, doorPrice: nil).priceText == "$20.50")
    }

    @Test func spellsAPairOutLoudForAScreenReader() throws {
        #expect(
            try show(price: 20, doorPrice: 25).priceAccessibilityLabel
                == "$20 advance, $25 at the door"
        )
    }

    @Test func leavesALonePriceUnlabelled() throws {
        #expect(try show(price: 20, doorPrice: nil).priceAccessibilityLabel == nil)
        #expect(try show(price: nil, doorPrice: nil).priceAccessibilityLabel == nil)
    }
}

@Suite("Show form price parsing")
struct ShowFormPriceParsingTests {
    @Test func readsAPlainAmount() {
        #expect(ShowFormView.parsePrice("20") == 20)
        #expect(ShowFormView.parsePrice("$25") == 25)
        #expect(ShowFormView.parsePrice(" $1,250 ") == 1250)
        #expect(ShowFormView.parsePrice("12.50") == 12.5)
    }

    @Test func readsAStatedZeroAsAPrice() {
        #expect(ShowFormView.parsePrice("0") == 0)
        #expect(ShowFormView.parsePrice("$0") == 0)
    }

    @Test func readsTheExtractorsWordForAFreeShow() {
        #expect(ShowFormView.parsePrice("Free") == 0)
        #expect(ShowFormView.parsePrice("free") == 0)
    }

    @Test func readsAnEmptyFieldAsNoPrice() {
        #expect(ShowFormView.parsePrice("") == nil)
        #expect(ShowFormView.parsePrice("   ") == nil)
        #expect(ShowFormView.parsePrice("call venue") == nil)
    }

    @Test func refusesNonFiniteAmounts() {
        // JSONSerialization raises an Objective-C exception on these, which no
        // Swift catch can take.
        #expect(ShowFormView.parsePrice("nan") == nil)
        #expect(ShowFormView.parsePrice("inf") == nil)
        #expect(ShowFormView.parsePrice("-inf") == nil)
        #expect(ShowFormView.parsePrice("infinity") == nil)
    }
}
