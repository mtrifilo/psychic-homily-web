import type { EntityType } from "./types";

export interface ValidationError {
  field: string;
  message: string;
}

export interface ValidationResult {
  valid: boolean;
  errors: ValidationError[];
}

function isNonEmptyString(value: unknown): boolean {
  return typeof value === "string" && value.trim().length > 0;
}

function isNonEmptyArray(value: unknown): boolean {
  return Array.isArray(value) && value.length > 0;
}

function requireField(
  data: Record<string, unknown>,
  field: string,
  errors: ValidationError[],
  message?: string,
): void {
  if (!isNonEmptyString(data[field])) {
    errors.push({
      field,
      message: message || `${field} is required`,
    });
  }
}

/** Validate artist data. Required: name. */
export function validateArtist(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "name", errors);

  return { valid: errors.length === 0, errors };
}

/** Validate venue data. Required: name, city, state. */
export function validateVenue(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "name", errors);
  requireField(d, "city", errors);
  requireField(d, "state", errors);

  return { valid: errors.length === 0, errors };
}

/** Validate show data. Required: event_date, city, state, at least 1 artist and 1 venue. */
export function validateShow(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "event_date", errors);
  requireField(d, "city", errors);
  requireField(d, "state", errors);

  if (!isNonEmptyArray(d.artists)) {
    errors.push({ field: "artists", message: "At least one artist is required" });
  }

  if (!isNonEmptyArray(d.venues)) {
    errors.push({ field: "venues", message: "At least one venue is required" });
  }

  return { valid: errors.length === 0, errors };
}

/** Validate release data. Required: title, at least 1 artist. */
export function validateRelease(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "title", errors);

  if (!isNonEmptyArray(d.artists)) {
    errors.push({ field: "artists", message: "At least one artist is required" });
  }

  return { valid: errors.length === 0, errors };
}

/** Validate label data. Required: name. */
export function validateLabel(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "name", errors);

  return { valid: errors.length === 0, errors };
}

/** Validate festival data. Required: name, series_slug, edition_year, start_date, end_date. */
export function validateFestival(data: unknown): ValidationResult {
  const errors: ValidationError[] = [];

  if (!data || typeof data !== "object") {
    return { valid: false, errors: [{ field: "_root", message: "Data must be an object" }] };
  }

  const d = data as Record<string, unknown>;
  requireField(d, "name", errors);
  requireField(d, "series_slug", errors);

  if (d.edition_year === undefined || d.edition_year === null || d.edition_year === "") {
    errors.push({ field: "edition_year", message: "edition_year is required" });
  }

  requireField(d, "start_date", errors);
  requireField(d, "end_date", errors);

  return { valid: errors.length === 0, errors };
}

/** Validate any entity by type. */
export function validateEntity(entityType: EntityType, data: unknown): ValidationResult {
  switch (entityType) {
    case "artist":
      return validateArtist(data);
    case "venue":
      return validateVenue(data);
    case "show":
      return validateShow(data);
    case "release":
      return validateRelease(data);
    case "label":
      return validateLabel(data);
    case "festival":
      return validateFestival(data);
    default: {
      const _exhaustive: never = entityType;
      return { valid: false, errors: [{ field: "_root", message: `Unknown entity type: ${_exhaustive}` }] };
    }
  }
}
