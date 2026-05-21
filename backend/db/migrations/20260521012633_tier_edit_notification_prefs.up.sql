-- PSY-756: per-category notification preferences for tier-change and
-- edit-review emails.
--
-- Five notification senders (tier promotion/demotion/demotion-warning, edit
-- approved/rejected) lacked RFC 8058 List-Unsubscribe headers and an in-body
-- opt-out. Wiring those requires a preference flag the unsubscribe link can
-- flip and the senders can gate on.
--
-- Two columns (per-category), matching the locked decision to let users opt
-- out of tier-change emails separately from edit-review emails:
--
--   * notify_on_tier_notifications  — promotion / demotion / demotion-warning
--   * notify_on_edit_notifications  — edit approved / rejected
--
-- Both default TRUE (opt-OUT), matching notify_on_comment_subscription and
-- notify_on_mention: each is a single email triggered by one discrete action
-- the user took (submitting an edit) or that directly concerns their account
-- (a tier change). This is the standard opt-OUT default per project policy
-- (feedback_default_optout_notifications). The collection-digest opt-IN
-- (DEFAULT FALSE) stays the documented exception — it is the only firehose
-- that can deliver many emails with no direct interaction.

ALTER TABLE user_preferences
    ADD COLUMN notify_on_tier_notifications BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN notify_on_edit_notifications BOOLEAN NOT NULL DEFAULT TRUE;
