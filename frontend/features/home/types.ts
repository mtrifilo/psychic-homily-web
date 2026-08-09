/** GET /community/pulse — homepage global heartbeat (PSY-1431). */
export interface CommunityPulseResponse {
  /**
   * Approved, non-cancelled shows in a ROLLING `[start-of-today, +7d)` window,
   * NOT a Monday-to-Sunday week. Its own field, unrelated to
   * `SceneListItem.shows_this_week`, but it carries the same trap and the same
   * rule: anything rendered from it is worded "next 7 days" (PSY-1732), never
   * "this week".
   */
  shows_this_week: number
  entities_in_graph: number
}
