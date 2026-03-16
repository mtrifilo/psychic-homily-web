import { describe, test, expect } from "bun:test";
import {
  validateArtist,
  validateVenue,
  validateShow,
  validateRelease,
  validateLabel,
  validateFestival,
  validateEntity,
} from "../src/lib/schemas";

describe("validateArtist", () => {
  test("passes with valid data", () => {
    const result = validateArtist({ name: "Radiohead" });
    expect(result.valid).toBe(true);
    expect(result.errors).toHaveLength(0);
  });

  test("fails when name is missing", () => {
    const result = validateArtist({});
    expect(result.valid).toBe(false);
    expect(result.errors).toHaveLength(1);
    expect(result.errors[0].field).toBe("name");
  });

  test("fails when name is empty string", () => {
    const result = validateArtist({ name: "" });
    expect(result.valid).toBe(false);
    expect(result.errors[0].field).toBe("name");
  });

  test("fails when name is whitespace only", () => {
    const result = validateArtist({ name: "   " });
    expect(result.valid).toBe(false);
  });

  test("fails when data is null", () => {
    const result = validateArtist(null);
    expect(result.valid).toBe(false);
    expect(result.errors[0].field).toBe("_root");
  });

  test("fails when data is not an object", () => {
    const result = validateArtist("string");
    expect(result.valid).toBe(false);
  });

  test("passes with extra fields", () => {
    const result = validateArtist({ name: "Radiohead", city: "Oxford", website: "https://radiohead.com" });
    expect(result.valid).toBe(true);
  });
});

describe("validateVenue", () => {
  test("passes with valid data", () => {
    const result = validateVenue({ name: "The Rebel Lounge", city: "Phoenix", state: "AZ" });
    expect(result.valid).toBe(true);
    expect(result.errors).toHaveLength(0);
  });

  test("fails when name is missing", () => {
    const result = validateVenue({ city: "Phoenix", state: "AZ" });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "name")).toBe(true);
  });

  test("fails when city is missing", () => {
    const result = validateVenue({ name: "The Rebel Lounge", state: "AZ" });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "city")).toBe(true);
  });

  test("fails when state is missing", () => {
    const result = validateVenue({ name: "The Rebel Lounge", city: "Phoenix" });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "state")).toBe(true);
  });

  test("fails when all required fields missing", () => {
    const result = validateVenue({});
    expect(result.valid).toBe(false);
    expect(result.errors).toHaveLength(3);
  });
});

describe("validateShow", () => {
  test("passes with valid data", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      city: "Phoenix",
      state: "AZ",
      artists: ["Radiohead"],
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(true);
  });

  test("fails when event_date is missing", () => {
    const result = validateShow({
      city: "Phoenix",
      state: "AZ",
      artists: ["Radiohead"],
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "event_date")).toBe(true);
  });

  test("fails when city is missing", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      state: "AZ",
      artists: ["Radiohead"],
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "city")).toBe(true);
  });

  test("fails when state is missing", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      city: "Phoenix",
      artists: ["Radiohead"],
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "state")).toBe(true);
  });

  test("fails when artists array is empty", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      city: "Phoenix",
      state: "AZ",
      artists: [],
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "artists")).toBe(true);
  });

  test("fails when artists is missing", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      city: "Phoenix",
      state: "AZ",
      venues: ["The Rebel Lounge"],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "artists")).toBe(true);
  });

  test("fails when venues array is empty", () => {
    const result = validateShow({
      event_date: "2026-03-15",
      city: "Phoenix",
      state: "AZ",
      artists: ["Radiohead"],
      venues: [],
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "venues")).toBe(true);
  });

  test("fails when all required fields are missing", () => {
    const result = validateShow({});
    expect(result.valid).toBe(false);
    expect(result.errors).toHaveLength(5);
  });
});

describe("validateRelease", () => {
  test("passes with valid data", () => {
    const result = validateRelease({ title: "OK Computer", artists: ["Radiohead"] });
    expect(result.valid).toBe(true);
  });

  test("fails when title is missing", () => {
    const result = validateRelease({ artists: ["Radiohead"] });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "title")).toBe(true);
  });

  test("fails when artists is missing", () => {
    const result = validateRelease({ title: "OK Computer" });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "artists")).toBe(true);
  });

  test("fails when both missing", () => {
    const result = validateRelease({});
    expect(result.valid).toBe(false);
    expect(result.errors).toHaveLength(2);
  });
});

describe("validateLabel", () => {
  test("passes with valid data", () => {
    const result = validateLabel({ name: "Sacred Bones Records" });
    expect(result.valid).toBe(true);
  });

  test("fails when name is missing", () => {
    const result = validateLabel({});
    expect(result.valid).toBe(false);
    expect(result.errors[0].field).toBe("name");
  });

  test("fails when name is empty", () => {
    const result = validateLabel({ name: "" });
    expect(result.valid).toBe(false);
  });
});

describe("validateFestival", () => {
  test("passes with valid data", () => {
    const result = validateFestival({
      name: "M3F Festival 2026",
      series_slug: "m3f-festival",
      edition_year: 2026,
      start_date: "2026-03-01",
      end_date: "2026-03-02",
    });
    expect(result.valid).toBe(true);
  });

  test("fails when name is missing", () => {
    const result = validateFestival({
      series_slug: "m3f-festival",
      edition_year: 2026,
      start_date: "2026-03-01",
      end_date: "2026-03-02",
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "name")).toBe(true);
  });

  test("fails when series_slug is missing", () => {
    const result = validateFestival({
      name: "M3F Festival 2026",
      edition_year: 2026,
      start_date: "2026-03-01",
      end_date: "2026-03-02",
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "series_slug")).toBe(true);
  });

  test("fails when edition_year is missing", () => {
    const result = validateFestival({
      name: "M3F Festival 2026",
      series_slug: "m3f-festival",
      start_date: "2026-03-01",
      end_date: "2026-03-02",
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "edition_year")).toBe(true);
  });

  test("fails when start_date is missing", () => {
    const result = validateFestival({
      name: "M3F Festival 2026",
      series_slug: "m3f-festival",
      edition_year: 2026,
      end_date: "2026-03-02",
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "start_date")).toBe(true);
  });

  test("fails when end_date is missing", () => {
    const result = validateFestival({
      name: "M3F Festival 2026",
      series_slug: "m3f-festival",
      edition_year: 2026,
      start_date: "2026-03-01",
    });
    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.field === "end_date")).toBe(true);
  });

  test("fails when all required fields are missing", () => {
    const result = validateFestival({});
    expect(result.valid).toBe(false);
    expect(result.errors).toHaveLength(5);
  });
});

describe("validateEntity", () => {
  test("dispatches to artist validator", () => {
    expect(validateEntity("artist", { name: "Test" }).valid).toBe(true);
    expect(validateEntity("artist", {}).valid).toBe(false);
  });

  test("dispatches to venue validator", () => {
    expect(validateEntity("venue", { name: "Test", city: "Phoenix", state: "AZ" }).valid).toBe(true);
    expect(validateEntity("venue", {}).valid).toBe(false);
  });

  test("dispatches to show validator", () => {
    expect(
      validateEntity("show", {
        event_date: "2026-03-15",
        city: "Phoenix",
        state: "AZ",
        artists: ["Test"],
        venues: ["Test"],
      }).valid,
    ).toBe(true);
    expect(validateEntity("show", {}).valid).toBe(false);
  });

  test("dispatches to release validator", () => {
    expect(validateEntity("release", { title: "Test", artists: ["Test"] }).valid).toBe(true);
    expect(validateEntity("release", {}).valid).toBe(false);
  });

  test("dispatches to label validator", () => {
    expect(validateEntity("label", { name: "Test" }).valid).toBe(true);
    expect(validateEntity("label", {}).valid).toBe(false);
  });

  test("dispatches to festival validator", () => {
    expect(
      validateEntity("festival", {
        name: "Test",
        series_slug: "test",
        edition_year: 2026,
        start_date: "2026-01-01",
        end_date: "2026-01-02",
      }).valid,
    ).toBe(true);
    expect(validateEntity("festival", {}).valid).toBe(false);
  });
});
