package notification

import (
	"fmt"
	"html/template"
	"strings"

	"psychic-homily-backend/internal/services/contracts"
)

// The HTML bodies of the pre-design-system emails, as html/template rather than
// fmt.Sprintf format strings.
//
// html/template escapes every action for the context it sits in: a text node,
// an attribute value, or the URL of an href. That is the property this file
// exists for. Show titles, venue and artist names, usernames, comment bodies and
// moderator reasons are all contributor-writable and reach these bodies
// verbatim, and these messages ship from the platform's own DKIM-aligned sender,
// so an unescaped value is a working link or a rewritten layout in the
// recipient's inbox.
//
// Escaping is a property of the renderer here, not of a call site, so a value
// added to a template is escaped whether or not the author thought about it.
// Nothing in this file escapes by hand, and no Go code in this package builds
// markup for these messages: the conditionals and lists that used to be
// concatenated into strings are {{if}} and {{range}} blocks below.
//
// The design-system messages (verification, artist and venue show alerts) render
// through the builders in email_layout.go instead, which escape every value at
// the point they write it. Those two are the only renderers in the package, and
// TestNoRawHTMLSprintf fails a third.

// sharedEmailBlocks are the fragments every legacy body is parsed with.
//
// A body names them rather than restating them, so the frame and the opt-out
// card each have one definition instead of one per message.
//
// legacyFrame{Open,Close} carry the parts identical in all of them: the doctype,
// the meta tags email clients need, the body's font stack and width, and the
// wordmark. What sits between them is what makes each message different.
//
// unsubscribeCard is the prominent in-body opt-out. Label describes the category
// in the recipient's words (e.g. "tier-change emails"); URL is the same
// HMAC-signed endpoint that goes in the List-Unsubscribe header, so a recipient
// and a mailbox provider have one way out, and it requires no login.
const sharedEmailBlocks = `{{define "legacyFrameOpen"}}
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #1a1a1a; margin: 0;">Psychic Homily</h1>
    </div>
{{end}}{{define "legacyFrameClose"}}</body>
</html>
{{end}}{{define "unsubscribeCard"}}
    <div style="background: #fff7ed; border: 1px solid #fed7aa; border-radius: 8px; padding: 16px 20px; margin-bottom: 20px;">
        <p style="margin: 0; font-size: 14px; color: #444;">
            Don&rsquo;t want {{.Label}}?
            <a href="{{.URL}}" style="color: #c2410c; font-weight: 600;">Unsubscribe in one click</a> &mdash;
            no login required.
        </p>
    </div>
{{end}}`

// unsubscribeCard is the data the shared opt-out block renders from.
type unsubscribeCard struct {
	URL   string
	Label string
}

// emailTemplateFuncs are the helpers the templates may call. Kept to the
// formatting the bodies genuinely need, since a func is the one way a value can
// reach a template without passing through the field it was declared for.
var emailTemplateFuncs = template.FuncMap{
	"pluralize": pluralize,
}

// mustEmailTemplate parses one email body together with the shared blocks.
// Parsing at package init means a malformed template stops the process at
// startup rather than at send time, when the failure would be a swallowed
// notification.
func mustEmailTemplate(name, body string) *template.Template {
	return template.Must(
		template.New(name).Funcs(emailTemplateFuncs).Parse(sharedEmailBlocks + body),
	)
}

// renderEmailTemplate executes one email body against its data.
//
// The error can only be a template that disagrees with its data struct, which
// every template's test would catch; it is returned rather than panicked so a
// send path fails the one message instead of the process.
func renderEmailTemplate(tmpl *template.Template, data any) (string, error) {
	// Bodies run 2-3KB, so one sized allocation replaces the handful of
	// doublings a zero-value builder would take to get there.
	var b strings.Builder
	b.Grow(4096)
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render %s email body: %w", tmpl.Name(), err)
	}
	return b.String(), nil
}

// ──────────────────────────────────────────────
// Auth
// ──────────────────────────────────────────────

type magicLinkEmailData struct {
	MagicLinkURL string
}

var magicLinkEmailTemplate = mustEmailTemplate("magic_link", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Sign in to your account</h2>
        <p>Click the button below to sign in to your Psychic Homily account. This link will expire in 15 minutes.</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.MagicLinkURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Sign In</a>
        </p>
        <p style="font-size: 14px; color: #666;">For security, this link expires in 15 minutes and can only be used once.</p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>If you didn't request this email, you can safely ignore it.</p>
        <p>If the button doesn't work, copy and paste this link into your browser:</p>
        <p style="word-break: break-all; color: #666;">{{.MagicLinkURL}}</p>
    </div>
{{template "legacyFrameClose"}}`)

type accountRecoveryEmailData struct {
	DaysRemaining int
	RecoveryURL   string
}

var accountRecoveryEmailTemplate = mustEmailTemplate("account_recovery", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Recover Your Account</h2>
        <p>We received a request to recover your deleted Psychic Homily account. You have <strong>{{.DaysRemaining}} days remaining</strong> to recover your account before it is permanently deleted.</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.RecoveryURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Recover Account</a>
        </p>
        <p style="font-size: 14px; color: #666;">This link will expire in 1 hour.</p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>If you didn't request this, you can safely ignore this email. Your account will remain scheduled for deletion.</p>
        <p>If the button doesn't work, copy and paste this link into your browser:</p>
        <p style="word-break: break-all; color: #666;">{{.RecoveryURL}}</p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Show reminder
// ──────────────────────────────────────────────

type showReminderEmailData struct {
	ShowTitle      string
	FormattedDate  string
	VenueText      string
	ShowURL        string
	UnsubscribeURL string
}

var showReminderEmailTemplate = mustEmailTemplate("show_reminder", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">{{.ShowTitle}} is tomorrow!</h2>
        <p style="font-size: 16px; color: #444;">{{.FormattedDate}}</p>
        {{if .VenueText}}<p style="font-size: 16px; color: #444;">Venue: <strong>{{.VenueText}}</strong></p>{{end}}
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.ShowURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Show</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want these reminders? <a href="{{.UnsubscribeURL}}" style="color: #666;">Unsubscribe</a></p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Contributor tier changes
// ──────────────────────────────────────────────

type tierPromotionEmailData struct {
	Greeting       string
	OldDisplayName string
	DisplayName    string
	Reason         string
	NewTier        string
	NewPermissions []string
	Unsubscribe    unsubscribeCard
}

var tierPromotionEmailTemplate = mustEmailTemplate("tier_promotion", `{{template "legacyFrameOpen"}}
    <div style="background: #f0fdf4; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #bbf7d0;">
        <h2 style="margin-top: 0; color: #166534;">Congratulations, {{.Greeting}}!</h2>
        <p style="font-size: 16px;">You've been promoted from <strong>{{.OldDisplayName}}</strong> to <strong>{{.DisplayName}}</strong>.</p>
        <p style="color: #444;">{{.Reason}}</p>
        {{if .NewPermissions}}
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">New permissions unlocked:</h3>
        <ul style="padding-left: 20px; color: #444;">
            {{range .NewPermissions}}<li style="margin-bottom: 4px;">{{.}}</li>
            {{end}}
        </ul>
        {{end}}
        {{if eq .NewTier "contributor"}}
        <p style="font-size: 14px; color: #666; margin-top: 20px;">Keep contributing quality edits to reach <strong>Trusted Contributor</strong> status (25 approved edits with 95%+ approval rate).</p>
        {{else if eq .NewTier "trusted_contributor"}}
        <p style="font-size: 14px; color: #666; margin-top: 20px;">Keep contributing to your local scene to reach <strong>Local Ambassador</strong> status (50 approved edits with 10+ city edits).</p>
        {{else if eq .NewTier "local_ambassador"}}
        <p style="font-size: 14px; color: #666; margin-top: 20px;">You've reached the highest contributor tier. Thank you for your dedication to the community!</p>
        {{end}}
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Thank you for contributing to the Psychic Homily community.</p>
    </div>
{{template "legacyFrameClose"}}`)

type tierDemotionEmailData struct {
	Greeting       string
	OldDisplayName string
	NewDisplayName string
	Reason         string
	Unsubscribe    unsubscribeCard
}

var tierDemotionEmailTemplate = mustEmailTemplate("tier_demotion", `{{template "legacyFrameOpen"}}
    <div style="background: #fef9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #fecaca;">
        <h2 style="margin-top: 0; color: #991b1b;">Your contributor tier has changed</h2>
        <p>Hi {{.Greeting}},</p>
        <p>Your tier has changed from <strong>{{.OldDisplayName}}</strong> to <strong>{{.NewDisplayName}}</strong>.</p>
        <p style="color: #444;"><strong>Reason:</strong> {{.Reason}}</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">How to recover your tier:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Focus on submitting accurate, high-quality edits</li>
            <li style="margin-bottom: 4px;">Double-check your information before submitting</li>
            <li style="margin-bottom: 4px;">Review the contribution guidelines for best practices</li>
        </ul>
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Your contributions are valued. Keep at it and you'll regain your tier.</p>
    </div>
{{template "legacyFrameClose"}}`)

type tierDemotionWarningEmailData struct {
	Greeting    string
	CurrentRate float64
	Threshold   float64
	DisplayName string
	Unsubscribe unsubscribeCard
}

var tierDemotionWarningEmailTemplate = mustEmailTemplate("tier_demotion_warning", `{{template "legacyFrameOpen"}}
    <div style="background: #fffbeb; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #fde68a;">
        <h2 style="margin-top: 0; color: #92400e;">Your contributor status is at risk</h2>
        <p>Hi {{.Greeting}},</p>
        <p>Your current approval rate of <strong>{{printf "%.0f" .CurrentRate}}%</strong> is approaching the <strong>{{printf "%.0f" .Threshold}}%</strong> threshold required to maintain your <strong>{{.DisplayName}}</strong> status.</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">Tips to improve your approval rate:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Verify information from multiple sources before submitting</li>
            <li style="margin-bottom: 4px;">Pay attention to formatting and data accuracy</li>
            <li style="margin-bottom: 4px;">Review feedback on previously rejected edits</li>
        </ul>
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>This is a friendly heads-up to help you maintain your contributor status.</p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Edit review decisions
// ──────────────────────────────────────────────

type editApprovedEmailData struct {
	Greeting        string
	EntityType      string
	EntityName      string
	EntityURL       string
	EntityTypeTitle string
	Unsubscribe     unsubscribeCard
}

var editApprovedEmailTemplate = mustEmailTemplate("edit_approved", `{{template "legacyFrameOpen"}}
    <div style="background: #f0fdf4; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #bbf7d0;">
        <h2 style="margin-top: 0; color: #166534;">Your edit was approved!</h2>
        <p>Hi {{.Greeting}},</p>
        <p>Your edit to the {{.EntityType}} <strong>{{.EntityName}}</strong> has been reviewed and approved. Your changes are now live!</p>
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.EntityURL}}" style="display: inline-block; background: #16a34a; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View {{.EntityTypeTitle}}</a>
        </p>
        <p style="font-size: 14px; color: #444;">Thank you for improving the Psychic Homily database. Every contribution helps the community discover great music.</p>
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Keep contributing to build your reputation and unlock new permissions.</p>
    </div>
{{template "legacyFrameClose"}}`)

type editRejectedEmailData struct {
	EntityName      string
	Greeting        string
	EntityType      string
	RejectionReason string
	Unsubscribe     unsubscribeCard
}

var editRejectedEmailTemplate = mustEmailTemplate("edit_rejected", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px; border: 1px solid #e5e7eb;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Update on your edit to {{.EntityName}}</h2>
        <p>Hi {{.Greeting}},</p>
        <p>Your edit to the {{.EntityType}} <strong>{{.EntityName}}</strong> was not accepted this time.</p>
        <p style="background: #fef3c7; border-radius: 6px; padding: 12px 16px; color: #92400e;"><strong>Reason:</strong> {{.RejectionReason}}</p>
        <h3 style="color: #1a1a1a; margin-bottom: 8px;">Tips for future edits:</h3>
        <ul style="padding-left: 20px; color: #444;">
            <li style="margin-bottom: 4px;">Double-check facts against official sources (venue websites, artist pages)</li>
            <li style="margin-bottom: 4px;">Include a clear summary explaining why you are making the change</li>
            <li style="margin-bottom: 4px;">Ensure spelling and formatting are accurate</li>
        </ul>
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't be discouraged — your contributions are valued. Feel free to submit a revised edit.</p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Comments and mentions
// ──────────────────────────────────────────────

type commentNotificationEmailData struct {
	EntityName      string
	CommenterName   string
	EntityType      string
	CommentExcerpt  string
	EntityURL       string
	EntityTypeTitle string
	UnsubscribeURL  string
}

var commentNotificationEmailTemplate = mustEmailTemplate("comment_notification", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New comment on {{.EntityName}}</h2>
        <p style="font-size: 15px; color: #444;"><strong>{{.CommenterName}}</strong> commented on the {{.EntityType}} <strong>{{.EntityName}}</strong>:</p>
        <blockquote style="border-left: 4px solid #f97316; padding-left: 16px; margin: 16px 0; color: #555; font-style: italic;">{{.CommentExcerpt}}</blockquote>
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.EntityURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Discussion</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You're receiving this because you're subscribed to {{.EntityTypeTitle}} on {{.EntityName}}.</p>
        <p>Don't want these notifications? <a href="{{.UnsubscribeURL}}" style="color: #666;">Unsubscribe</a></p>
    </div>
{{template "legacyFrameClose"}}`)

type mentionNotificationEmailData struct {
	MentionerName  string
	EntityType     string
	EntityName     string
	CommentExcerpt string
	CommentURL     string
	UnsubscribeURL string
}

var mentionNotificationEmailTemplate = mustEmailTemplate("mention_notification", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">You were mentioned</h2>
        <p style="font-size: 15px; color: #444;"><strong>{{.MentionerName}}</strong> mentioned you in a comment on the {{.EntityType}} <strong>{{.EntityName}}</strong>:</p>
        <blockquote style="border-left: 4px solid #f97316; padding-left: 16px; margin: 16px 0; color: #555; font-style: italic;">{{.CommentExcerpt}}</blockquote>
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.CommentURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">Reply</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want mention notifications? <a href="{{.UnsubscribeURL}}" style="color: #666;">Unsubscribe</a></p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Digests
// ──────────────────────────────────────────────

type collectionDigestEmailData struct {
	Groups      []contracts.CollectionDigestGroup
	Unsubscribe unsubscribeCard
	FrontendURL string
}

var collectionDigestEmailTemplate = mustEmailTemplate("collection_digest", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New in your collections</h2>
        <p style="font-size: 15px; color: #444;">Items added to collections you follow over the past week.</p>
        {{range .Groups}}
        <div style="margin-bottom: 24px;">
            <h3 style="margin: 0 0 8px; color: #1a1a1a;"><a href="{{.CollectionURL}}" style="color: #1a1a1a; text-decoration: none;">{{.CollectionTitle}}</a></h3>
            <ul style="margin: 0; padding-left: 20px; color: #444;">
                {{range .Items}}<li style="margin-bottom: 4px;"><a href="{{.EntityURL}}" style="color: #f97316; text-decoration: none;">{{.EntityName}}</a> <span style="color: #888;">({{.EntityType}}, added by {{.AddedBy}})</span></li>
                {{end}}
            </ul>
        </div>
        {{end}}
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You&rsquo;re receiving this because you opted in to weekly digests for collections you follow on Psychic Homily.</p>
        <p>Manage all notifications in your <a href="{{.FrontendURL}}/settings" style="color: #666;">notification settings</a>.</p>
    </div>
{{template "legacyFrameClose"}}`)

type sceneDigestEmailData struct {
	Groups      []contracts.SceneDigestGroup
	Unsubscribe unsubscribeCard
	FrontendURL string
}

// The "+N more" line and the two section headings sit inside {{range .Groups}}
// but outside {{range .Shows}} / {{range .NewArtists}}, so their dot is the
// GROUP. A heading moved inside an inner range would repeat per row, and
// MoreNewArtists is not a field of an artist at all.
var sceneDigestEmailTemplate = mustEmailTemplate("scene_digest", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">Your scenes: the next 7 days</h2>
        <p style="font-size: 15px; color: #444;">Shows in the next 7 days and new bands, for the scenes you follow.</p>
        {{range .Groups}}
        <div style="margin-bottom: 28px;">
            <h3 style="margin: 0 0 8px; color: #1a1a1a;"><a href="{{.SceneURL}}" style="color: #1a1a1a; text-decoration: none;">{{.SceneName}}</a></h3>
            {{if .Shows}}
            <p style="margin: 4px 0; font-size: 13px; font-weight: 600; color: #666; text-transform: uppercase; letter-spacing: 0.04em;">Next 7 days</p>
            <ul style="margin: 0 0 10px; padding-left: 20px; color: #444;">
                {{range .Shows}}<li style="margin-bottom: 4px;"><a href="{{.ShowURL}}" style="color: #f97316; text-decoration: none;">{{.DisplayTitle}}</a> <span style="color: #888;">({{.Date}}{{if .VenueName}} · {{.VenueName}}{{end}})</span></li>
                {{end}}
            </ul>
            {{end}}
            {{if .NewArtists}}
            <p style="margin: 4px 0; font-size: 13px; font-weight: 600; color: #666; text-transform: uppercase; letter-spacing: 0.04em;">New bands based here</p>
            <ul style="margin: 0; padding-left: 20px; color: #444;">
                {{range .NewArtists}}<li style="margin-bottom: 4px;"><a href="{{.ArtistURL}}" style="color: #f97316; text-decoration: none;">{{.Name}}</a></li>
                {{end}}
                {{if gt .MoreNewArtists 0}}<li style="margin-bottom: 4px; list-style: none; color: #888;"><a href="{{.SceneURL}}" style="color: #888;">+{{.MoreNewArtists}} more new {{pluralize "band" .MoreNewArtists}} — see the scene</a></li>
                {{end}}
            </ul>
            {{end}}
        </div>
        {{end}}
    </div>
{{template "unsubscribeCard" .Unsubscribe}}
    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>You&rsquo;re receiving this because you opted in to weekly scene digests on Psychic Homily.</p>
        <p>Manage all notifications in your <a href="{{.FrontendURL}}/settings" style="color: #666;">notification settings</a>.</p>
    </div>
{{template "legacyFrameClose"}}`)

// ──────────────────────────────────────────────
// Filter and scene-follow match
// ──────────────────────────────────────────────

type filterEmailData struct {
	FilterName     string
	ShowTitle      string
	ShowDate       string
	VenueText      string
	ArtistText     string
	PriceText      string
	ShowURL        string
	UnsubscribeURL string
}

var filterEmailTemplate = mustEmailTemplate("filter_match", `{{template "legacyFrameOpen"}}
    <div style="background: #f9f9f9; border-radius: 8px; padding: 30px; margin-bottom: 20px;">
        <h2 style="margin-top: 0; color: #1a1a1a;">New show matching "{{.FilterName}}"</h2>
        <p style="font-size: 18px; font-weight: 600; color: #1a1a1a; margin: 8px 0;">{{.ShowTitle}}</p>
        <p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Date:</strong> {{.ShowDate}}</p>
        {{if .VenueText}}<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Venue:</strong> {{.VenueText}}</p>{{end}}
        {{if .ArtistText}}<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Artists:</strong> {{.ArtistText}}</p>{{end}}
        {{if .PriceText}}<p style="font-size: 15px; color: #444; margin: 4px 0;"><strong>Price:</strong> {{.PriceText}}</p>{{end}}
        <p style="text-align: center; margin: 30px 0;">
            <a href="{{.ShowURL}}" style="display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 30px; border-radius: 6px; font-weight: 600;">View Show</a>
        </p>
    </div>

    <div style="text-align: center; font-size: 12px; color: #999;">
        <p>Don't want these notifications? <a href="{{.UnsubscribeURL}}" style="color: #666;">Pause this filter</a></p>
    </div>
{{template "legacyFrameClose"}}`)
