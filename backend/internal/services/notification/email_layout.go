package notification

import (
	"fmt"
	"strings"
)

// Design-system colors, inlined as literal hex.
//
// Email clients cannot resolve CSS custom properties, so the light-mode values
// of the frontend's `--background`, `--foreground`, `--muted-foreground`,
// `--border`, `--primary` and `--primary-foreground` tokens are duplicated
// here. They must be kept in sync with frontend/app/globals.css by hand.
const (
	emailBackground        = "#f4f1ea"
	emailForeground        = "#1a1714"
	emailMutedForeground   = "#6b5e4f"
	emailBorder            = "#cabe9f"
	emailPrimary           = "#d2541b"
	emailPrimaryForeground = "#f4f1ea"
)

// Font stacks.
//
// The site loads Clash Display / Satoshi / Space Mono as webfonts. Email
// clients strip @font-face, so these are the closest widely-installed
// fallbacks: a neutral grotesk for prose and a monospace for the metadata
// lines. Space Mono leads the mono stack for the minority of recipients who
// happen to have it installed locally.
const (
	emailSansStack = "'Helvetica Neue', Helvetica, Arial, sans-serif"
	emailMonoStack = "'Space Mono', 'SFMono-Regular', Menlo, Consolas, 'Liberation Mono', monospace"
)

// emailRowGap is the vertical rhythm between blocks inside the frame.
//
// Every block carries this as bottom padding, including the last one, which is
// why emailShell reserves only 20px of its own bottom padding: the trailing
// block's gap plus the shell's padding add up to the 40px the design asks for.
// Uniform rows mean a caller can append or reorder blocks without special
// casing the final one.
const emailRowGap = "20px"

// emailShell wraps message blocks in the shared frame: masthead, kicker rule,
// hairline border, and the table scaffolding email clients need.
//
// kicker is plain text (it is escaped here); bodyRows is already-built HTML
// from the emailHeadline / emailParagraph / emailButton / emailMonoNote /
// emailFineprint helpers, each of which emits one or more complete <tr>
// elements.
//
// The layout is nested tables rather than flexbox because Outlook renders mail
// through the Word engine, which supports neither flexbox nor grid.
//
// The frame is a fixed 600px inline, and only the <style> block widens it to
// 100% and tightens the padding below 600px. Doing it in that order rather than
// declaring width:100% inline is what keeps Outlook correct: the Word engine
// ignores max-width, so an inline width:100% would override the width="600"
// attribute sitting beside it and stretch the column across a maximized window.
// The same engine ignores <style> entirely, which is harmless here because the
// only rules in it are the ones a desktop window never needs.
//
// color-scheme is declared "light" because only the light palette ships here; a
// dark variant belongs with the suite-wide restyle, not with one message.
// Clients that honor the declaration leave the palette alone. Clients that
// force-invert anyway (Gmail on Android) still produce a readable message,
// because every text-bearing element sets its own color AND background-color
// rather than inheriting one of them, so inversion moves each pair together and
// cannot land text on a same-colored surface.
func emailShell(kicker, bodyRows string) string {
	return fmt.Sprintf(`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<meta name="color-scheme" content="light" />
<meta name="supported-color-schemes" content="light" />
<style type="text/css">
@media only screen and (max-width: 599px) {
  .ph-frame { width: 100%% !important; }
  .ph-frame-pad { padding: 32px 24px 20px 24px !important; }
}
</style>
</head>
<body style="margin:0; padding:0; background-color:%[1]s;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="width:100%%; background-color:%[1]s;">
<tr>
<td align="center" style="padding:0;">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" class="ph-frame" style="width:600px; max-width:600px; background-color:%[1]s; border:1px solid %[2]s;">
<tr>
<td class="ph-frame-pad" style="padding:48px 48px 20px 48px;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="width:100%%;">
%[3]s%[4]s</table>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>
`, emailBackground, emailBorder, emailMasthead(kicker), bodyRows)
}

// emailMasthead renders the wordmark plus the kicker rule: the label, then a
// hairline that runs out to the right edge.
//
// The hairline is a 1px-tall table cell with a background color rather than a
// border, because Outlook drops borders on zero-height elements. The label cell
// is width="1%" + nowrap so the table gives it exactly its intrinsic width and
// hands the remainder to the rule.
//
// nowrap makes the kicker the table's minimum width, so it has a length budget:
// roughly 35 characters, which is about 255px at 11px mono plus the 12px gutter,
// against the ~270px a 320px phone leaves after the mobile padding. Past that
// the frame starts scrolling sideways. Keep new kickers at or under the length
// of "YOUR ACCOUNT · PENDING VERIFICATION" or give the rule its own wrapping
// treatment first.
func emailMasthead(kicker string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[7]s 0; font-family:%[1]s; font-size:22px; font-weight:700; line-height:27px; letter-spacing:3.08px; color:%[2]s; background-color:%[8]s;">PSYCHIC HOMILY</td>
</tr>
<tr>
<td style="padding:0 0 %[7]s 0;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="width:100%%;">
<tr>
<td width="1%%" style="padding:0; font-family:%[3]s; font-size:11px; line-height:16px; letter-spacing:0.66px; color:%[4]s; background-color:%[8]s; white-space:nowrap;">%[5]s</td>
<td width="99%%" style="padding:0 0 0 12px;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="width:100%%;">
<tr><td height="1" style="height:1px; line-height:1px; font-size:1px; background-color:%[6]s;">&nbsp;</td></tr>
</table>
</td>
</tr>
</table>
</td>
</tr>
`, emailSansStack, emailForeground, emailMonoStack, emailMutedForeground, htmlEscape(kicker), emailBorder, emailRowGap, emailBackground)
}

// emailHeadline renders the message's one-line statement of what happened.
func emailHeadline(text string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:30px; font-weight:700; line-height:36px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailSansStack, emailForeground, htmlEscape(text), emailRowGap, emailBackground)
}

// emailSectionLabel renders a small mono label that names the block beneath it
// (PSY-1897, Figma 1584:2).
//
// It exists because a message can now be about several SUBJECTS at once, not
// just several items. The venue digest lists shows that all share a venue, so
// one headline plus one list says everything; a release roundup spans artists,
// and the reader's actual question — "which of my bands put something out" — is
// answered by the grouping before they read a single title.
//
// Styled from the masthead kicker rather than as a small headline, deliberately:
// it is a signpost, not a second statement of what happened, and borrowing the
// kicker's mono/tracked/muted treatment keeps the message's one bold voice
// belonging to emailHeadline.
//
// The bottom padding is 8px rather than emailRowGap because this block is
// ATTACHED to what follows it. A label floating a full rhythm above its list
// would read as belonging to the paragraph above instead.
func emailSectionLabel(text string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:8px 0 8px 0; font-family:%[1]s; font-size:11px; font-weight:400; letter-spacing:0.06em; line-height:16px; color:%[2]s; background-color:%[4]s;">%[3]s</td>
</tr>
`, emailMonoStack, emailMutedForeground, htmlEscape(strings.ToUpper(text)), emailBackground)
}

// emailParagraph renders a block of body prose.
func emailParagraph(text string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:15px; font-weight:400; line-height:24px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailSansStack, emailForeground, htmlEscape(text), emailRowGap, emailBackground)
}

// emailButton renders the primary call to action.
//
// The colored surface is a table cell rather than the anchor's own background,
// and the anchor repeats the radius and background so its label never ends up on
// an undeclared surface.
//
// The button's shape is declared twice on purpose. The anchor's padding gives
// every standards-based client a full-size click target. Outlook's Word engine
// drops padding on inline elements, which would collapse the painted chip to the
// text box, so the cell restates the same inset as mso-padding-alt, a property
// only that engine reads.
func emailButton(href, label string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[6]s 0;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0">
<tr>
<td bgcolor="%[1]s" style="background-color:%[1]s; border-radius:2px; mso-padding-alt:13px 28px;">
<a href="%[4]s" style="display:inline-block; padding:13px 28px; border-radius:2px; background-color:%[1]s; font-family:%[3]s; font-size:15px; font-weight:600; line-height:18px; color:%[2]s; text-decoration:none;">%[5]s</a>
</td>
</tr>
</table>
</td>
</tr>
`, emailPrimary, emailPrimaryForeground, emailSansStack, htmlEscape(href), htmlEscape(label), emailRowGap)
}

// emailMonoNote renders a small uppercase monospace detail line, used for
// facts about the message itself such as link expiry.
func emailMonoNote(text string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:11px; line-height:16px; letter-spacing:0.44px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailMonoStack, emailMutedForeground, htmlEscape(text), emailRowGap, emailBackground)
}

// sanitizeEmailHeaderValue strips the characters that END a header line, so a
// caller-supplied string cannot inject one.
//
// It exists because escaping is medium-specific and this repo's escaping is all
// aimed at the BODY: htmlEscape makes a scraped artist name safe inside markup
// and does nothing at all for a Subject, where the dangerous character is not
// `<` but a newline. Everything an ingest scrape produces is caller-supplied in
// that sense, and these messages ship from the platform's own DKIM-aligned
// sender.
//
// Removed rather than replaced: a subject is a single line by definition, so
// there is no rendering to preserve, and a substitute character would only
// invite the question of which one.
func sanitizeEmailHeaderValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, value)
}

// emailMonoDetails renders the mono DETAILS BLOCK: several aligned metadata
// lines about the thing the message is announcing, such as when, where, and who
// else is on the bill.
//
// Separate from emailMonoNote rather than folded into it, because the two carry
// different content at the same size: emailMonoNote is one fact about the
// MESSAGE ("this link expires in 24 hours"), and this is a small table of facts
// about the SUBJECT. Folding them together would mean passing a single string
// with newlines, and a newline is whitespace in HTML: every line would run
// together into one paragraph. Each line gets its own block element instead.
//
// Callers pass lines already padded to align their values (WHEN ...., WHERE ...)
// which only lands because the stack is monospace. white-space:pre keeps the
// runs of padding the caller wrote, since HTML would otherwise collapse them to
// a single space and the alignment would be lost in every client.
func emailMonoDetails(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, `<div style="white-space:pre;">%s</div>`, htmlEscape(line))
	}
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:12px; line-height:20px; letter-spacing:0.44px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailMonoStack, emailForeground, b.String(), emailRowGap, emailBackground)
}

// maxEmailSubjectEntityRunes bounds a scraped entity name interpolated into a
// Subject header.
//
// Sanitizing the name stops header SPLITTING; it does nothing about length, and
// an unfolded multi-kilobyte subject is provider-dependent behaviour ranging
// from truncation to rejection. A rejected send on the alert paths is logged and
// swallowed, so the failure mode is a silently lost notification.
//
// 120 is well past what any client displays (the masthead's own budget is ~35
// characters) and well short of the 998-octet line limit, leaving room for the
// rest of the subject.
const maxEmailSubjectEntityRunes = 120

// truncateRunes shortens a string to at most n RUNES, appending an ellipsis when
// it cuts. Runes, not bytes, so a multi-byte name cannot be sliced mid-character
// into invalid UTF-8.
func truncateRunes(value string, n int) string {
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[:n]) + "…"
}

// emailListRow is one entry in an emailListRows table: a short left-hand label,
// a title, and an optional secondary detail line under the title.
type emailListRow struct {
	// Label is the mono left column, e.g. a date. Kept short — it is given a
	// fixed 110px column, which fits about twelve monospace characters.
	Label string
	// Title is the entry's headline, e.g. a show title.
	Title string
	// Detail is the optional second line under the title, e.g. a bill. An empty
	// Detail renders no line rather than an empty one.
	Detail string
	// Href, when set, makes the Title a link to this destination (PSY-1897).
	//
	// A STRUCTURED parameter rather than letting callers pass markup, for the same
	// reason emailFineprintWithLinks takes one: every field in this struct is
	// escaped by the renderer, so a caller-built <a> would arrive as visible angle
	// brackets, and loosening the escaping to allow it would loosen it for the
	// scraped third-party text these rows usually carry.
	//
	// Empty leaves the title as plain text, byte for byte what it was before this
	// field existed — which is what keeps the venue digest (PSY-1895), whose rows
	// share one destination and use the CTA button for it, unchanged.
	// TestEmailListRowsWithoutHrefIsUnchanged pins that.
	Href string
}

// emailListRows renders a hairline-separated TABLE of entries: the block a
// message uses when it is announcing SEVERAL things at once.
//
// This is the one layout element the digest could not inherit (PSY-1895, Figma
// 1577:27). emailMonoDetails describes ONE subject as aligned WHEN/WHERE/WITH
// lines; three of those stacked reads as three messages stapled together rather
// than as one list, and its white-space:pre alignment only survives while every
// value is short.
//
// A real <table> with per-row border-top rather than div borders, because the
// Word engine Outlook renders through drops borders on block elements but
// honours them on table cells. The last row also carries a border-bottom, which
// is what makes the block read as terminated instead of running into the
// paragraph beneath it.
//
// Every field is escaped here. Callers pass plain text; a caller that needs a
// link inside a row would need a structured parameter, the same way
// emailFineprintWithLinks takes one.
func emailListRows(rows []emailListRow) string {
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	for i, row := range rows {
		// Every row draws its own top rule; only the last draws a bottom one, so
		// adjacent rows never double up into a 2px rule.
		bottom := "0"
		if i == len(rows)-1 {
			bottom = "1px"
		}

		detail := ""
		if row.Detail != "" {
			detail = fmt.Sprintf(
				`<div style="font-family:%[1]s; font-size:13px; line-height:19px; color:%[2]s;">%[3]s</div>`,
				emailSansStack, emailMutedForeground, htmlEscape(row.Detail))
		}

		// The title is wrapped in an anchor only when the caller supplied a
		// destination. The anchor carries its own color and text-decoration:
		// several clients (Gmail on iOS especially) restyle bare links to a blue
		// underline that would break the type ramp, and an inline declaration is
		// the only place they honour.
		title := htmlEscape(row.Title)
		if row.Href != "" {
			title = fmt.Sprintf(
				`<a href="%s" style="color:%s; text-decoration:none;">%s</a>`,
				htmlEscape(row.Href), emailForeground, title)
		}

		fmt.Fprintf(&b, `<tr>
<td width="110" valign="top" style="width:110px; padding:12px 16px 12px 0; border-top:1px solid %[1]s; border-bottom:%[6]s solid %[1]s; font-family:%[2]s; font-size:13px; line-height:20px; color:%[3]s; background-color:%[7]s; white-space:nowrap;">%[4]s</td>
<td valign="top" style="padding:12px 0 12px 0; border-top:1px solid %[1]s; border-bottom:%[6]s solid %[1]s; background-color:%[7]s;"><div style="font-family:%[8]s; font-size:14px; font-weight:600; line-height:20px; color:%[3]s;">%[5]s</div>%[9]s</td>
</tr>
`, emailBorder, emailMonoStack, emailForeground, htmlEscape(row.Label), title,
			bottom, emailBackground, emailSansStack, detail)
	}

	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[2]s 0;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="width:100%%; border-collapse:collapse;">
%[1]s</table>
</td>
</tr>
`, b.String(), emailRowGap)
}

// emailFineprintLink is one labelled destination in a fineprint footer.
type emailFineprintLink struct {
	Href  string
	Label string
}

// emailFineprintWithLinks renders the closing block for a message that has to
// offer the recipient a way out: the "why you are getting this" line, then a row
// of anchors.
//
// emailFineprint cannot do this. It escapes every line it is given, which is
// correct for prose and fatal for markup, so an unsubscribe link passed through
// it arrives as visible angle brackets. Rather than loosen that escaping — the
// one thing standing between a scraped venue-calendar string and a working
// phishing link in this platform's own DKIM-aligned mail — the links are a
// separate, structured parameter whose href and label are escaped individually
// and whose <a> element this function alone controls.
//
// Every alert email needs this shape, so it lives beside the other builders
// instead of being spelled out at one call site: a footer with a working opt-out
// is what RFC 8058 compliance looks like in the body, and the next template must
// inherit it rather than reinvent it.
func emailFineprintWithLinks(lines []string, links []emailFineprintLink) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(emailFineprintLine(htmlEscape(line)))
	}
	if len(links) > 0 {
		var row strings.Builder
		for i, link := range links {
			if i > 0 {
				row.WriteString(" &middot; ")
			}
			// href and label are escaped INDIVIDUALLY and the anchor is written
			// here, so no caller can hand this function markup.
			fmt.Fprintf(&row, `<a href="%s" style="color:%s; text-decoration:underline;">%s</a>`,
				htmlEscape(link.Href), emailPrimary, htmlEscape(link.Label))
		}
		b.WriteString(emailFineprintLine(row.String()))
	}
	return emailFineprintRow(b.String())
}

// emailFineprintLine wraps one already-escaped fineprint line.
//
// The three wrap declarations are one behavior spelled three ways for three
// generations of client: overflow-wrap is the current property, word-break is
// the WebKit-era spelling, and word-wrap is the only one Outlook's Word engine
// reads. All of them keep a long token such as a tokenized URL inside the frame
// without breaking ordinary prose mid-word the way word-break:break-all would.
// Outlook matters most here, since this block exists for recipients whose client
// swallowed the button.
//
// Shared by both fineprint builders so the wrap trio has ONE definition. Two
// copies would drift, and the drift would only show up in the one client nobody
// tests in.
//
// content must already be escaped or deliberately-constructed markup; this
// function does not escape.
func emailFineprintLine(content string) string {
	return fmt.Sprintf(
		`<div style="word-wrap:break-word; word-break:break-word; overflow-wrap:anywhere;">%s</div>`,
		content)
}

// emailFineprintRow wraps assembled fineprint lines in the shared table row.
func emailFineprintRow(content string) string {
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:12px; font-weight:400; line-height:18px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailSansStack, emailMutedForeground, content, emailRowGap, emailBackground)
}

// emailFineprint renders the closing lines: the "not you" reassurance and the
// plain-link fallback for recipients whose client swallows the button.
//
// Escapes every line, so it cannot carry a link. A footer that needs a working
// opt-out anchor uses emailFineprintWithLinks, which takes its destinations as
// structured data rather than as markup.
func emailFineprint(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(emailFineprintLine(htmlEscape(line)))
	}
	return emailFineprintRow(b.String())
}
