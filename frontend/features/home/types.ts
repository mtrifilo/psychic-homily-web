/** GET /community/pulse — homepage global heartbeat (PSY-1431). */
export interface CommunityPulseResponse {
  shows_this_week: number
  entities_in_graph: number
}
