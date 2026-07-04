import type { APIClient } from "./api";

export interface RadioRematchResult {
  total: number;
  matched: number;
  unmatched: number;
  persist_errors?: number;
}

/** Rematch unmatched radio plays against the knowledge graph. */
export async function rematchRadioPlays(
  client: APIClient,
  opts?: { artistName?: string; labelName?: string },
): Promise<RadioRematchResult> {
  const body: Record<string, string> = {};
  if (opts?.artistName) body.artist_name = opts.artistName;
  if (opts?.labelName) body.label_name = opts.labelName;
  return client.post<RadioRematchResult>("/admin/radio/rematch", body);
}
