import { APIClient, APIError } from "./api";
import { normalizeForComparison, similarityScore } from "./duplicates";
import * as display from "./display";
import { green, yellow, gray, dim, cyan } from "./ansi";

/** Tag input as provided by the user — string (defaults to genre) or object. */
export type TagInput = string | { name: string; category?: string };

/** Normalized tag after parsing user input. */
export interface ParsedTag {
  name: string;
  category: string;
}

/** Result of resolving a tag against the database. */
export interface ResolvedTag {
  id: number;
  name: string;
  category: string;
  status: "exists" | "created" | "fuzzy_match";
  originalName?: string; // if fuzzy-matched, the user's original input
}

interface APITag {
  id: number;
  name: string;
  slug: string;
  category: string;
  usage_count?: number;
}

/**
 * TagResolver handles searching, creating, and applying tags with:
 * - Session cache to avoid duplicate API calls
 * - Fuzzy duplicate detection (warns on similar existing tags)
 * - Idempotent operations (409 on duplicate = success)
 */
export class TagResolver {
  private client: APIClient;
  private cache = new Map<string, ResolvedTag>();

  constructor(client: APIClient) {
    this.client = client;
  }

  /** Parse user-provided tag inputs into normalized form. */
  static parseTags(tags: TagInput[] | undefined): ParsedTag[] {
    if (!tags || tags.length === 0) return [];
    return tags.map((t) => {
      if (typeof t === "string") {
        return { name: t.trim(), category: "genre" };
      }
      return { name: t.name.trim(), category: t.category || "genre" };
    }).filter((t) => t.name.length > 0);
  }

  /**
   * Resolve all tags — search for existing, detect fuzzy duplicates,
   * prepare for creation. Does NOT create tags (that happens on confirm).
   */
  async resolveAll(tags: ParsedTag[]): Promise<ResolvedTag[]> {
    const results: ResolvedTag[] = [];

    for (const tag of tags) {
      const cacheKey = normalizeForComparison(tag.name);

      // Check session cache
      if (this.cache.has(cacheKey)) {
        results.push(this.cache.get(cacheKey)!);
        continue;
      }

      const resolved = await this.resolveOne(tag);
      this.cache.set(cacheKey, resolved);
      results.push(resolved);
    }

    return results;
  }

  /**
   * Ensure all tags exist (create if needed) and apply them to an entity.
   * Call this during --confirm after entity creation.
   */
  async applyToEntity(
    entityType: string,
    entityId: number,
    tags: ParsedTag[],
  ): Promise<{ applied: number; skipped: number; errors: number }> {
    let applied = 0;
    let skipped = 0;
    let errors = 0;

    for (const tag of tags) {
      try {
        // Ensure tag exists (create if needed)
        await this.ensureTagExists(tag);

        // Apply tag to entity by name (server resolves aliases automatically)
        await this.client.post(
          `/entities/${entityType}/${entityId}/tags`,
          { tag_name: tag.name },
        );
        applied++;
      } catch (err) {
        if (err instanceof APIError && err.status === 409) {
          // Already tagged — that's fine
          skipped++;
        } else {
          const msg = err instanceof Error ? err.message : "Unknown error";
          display.warn(`Failed to apply tag "${tag.name}": ${msg}`);
          errors++;
        }
      }
    }

    return { applied, skipped, errors };
  }

  /** Search for an existing tag, create if not found. Idempotent. */
  private async ensureTagExists(tag: ParsedTag): Promise<number> {
    const cacheKey = normalizeForComparison(tag.name);
    const cached = this.cache.get(cacheKey);
    if (cached && cached.status !== "created") {
      // Already exists in DB, no need to create
      return cached.id;
    }

    // Try to create — 409 means it already exists
    try {
      const result = await this.client.post<APITag>("/tags", {
        name: tag.name,
        category: tag.category,
      });
      const resolved: ResolvedTag = {
        id: result.id,
        name: result.name,
        category: result.category,
        status: "created",
      };
      this.cache.set(cacheKey, resolved);
      return result.id;
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        // Tag already exists — search for it to get the ID
        const existing = await this.searchExact(tag.name);
        if (existing) {
          const resolved: ResolvedTag = {
            id: existing.id,
            name: existing.name,
            category: existing.category,
            status: "exists",
          };
          this.cache.set(cacheKey, resolved);
          return existing.id;
        }
      }
      throw err;
    }
  }

  /** Resolve a single tag against the database. */
  private async resolveOne(tag: ParsedTag): Promise<ResolvedTag> {
    // Search for the tag
    const searchResults = await this.searchTags(tag.name);

    // Check for exact match (case-insensitive)
    const normalizedInput = normalizeForComparison(tag.name);
    const exactMatch = searchResults.find(
      (t) => normalizeForComparison(t.name) === normalizedInput,
    );

    if (exactMatch) {
      return {
        id: exactMatch.id,
        name: exactMatch.name,
        category: exactMatch.category,
        status: "exists",
      };
    }

    // Check for fuzzy matches (similar but not exact)
    const fuzzyMatch = searchResults.find(
      (t) => similarityScore(t.name, tag.name) >= 0.7,
    );

    if (fuzzyMatch) {
      return {
        id: fuzzyMatch.id,
        name: fuzzyMatch.name,
        category: fuzzyMatch.category,
        status: "fuzzy_match",
        originalName: tag.name,
      };
    }

    // No match — will need to be created
    return {
      id: 0,
      name: tag.name,
      category: tag.category,
      status: "created",
    };
  }

  private async searchTags(query: string): Promise<APITag[]> {
    try {
      const result = await this.client.get<{ tags: APITag[] }>(
        "/tags/search",
        { q: query },
      );
      return result.tags || [];
    } catch {
      return [];
    }
  }

  private async searchExact(name: string): Promise<APITag | null> {
    const results = await this.searchTags(name);
    const normalized = normalizeForComparison(name);
    return results.find(
      (t) => normalizeForComparison(t.name) === normalized,
    ) || null;
  }
}

/** Format tags for preview display. */
export function formatTagsPreview(resolved: ResolvedTag[]): string {
  if (resolved.length === 0) return "";

  const parts = resolved.map((t) => {
    const cat = dim(`(${t.category})`);
    switch (t.status) {
      case "exists":
        return `${t.name} ${cat}`;
      case "created":
        return `${t.name} ${cat} ${cyan("NEW")}`;
      case "fuzzy_match":
        return `${yellow(t.originalName || t.name)} → ${green(t.name)} ${cat} ${yellow("MATCHED")}`;
    }
  });

  return parts.join(", ");
}

/** Format a fuzzy match warning for display. */
export function formatFuzzyWarning(resolved: ResolvedTag): string {
  if (resolved.status !== "fuzzy_match") return "";
  return `Tag "${resolved.originalName}" is similar to existing "${resolved.name}" — will use "${resolved.name}"`;
}
