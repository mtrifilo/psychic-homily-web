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

// emailFineprint renders the closing lines: the "not you" reassurance and the
// plain-link fallback for recipients whose client swallows the button.
//
// The three wrap declarations are one behavior spelled three ways for three
// generations of client: overflow-wrap is the current property, word-break is
// the WebKit-era spelling, and word-wrap is the only one Outlook's Word engine
// reads. All of them keep a long token such as a tokenized URL inside the frame
// without breaking ordinary prose mid-word the way word-break:break-all would.
// Outlook matters most here, since this block exists for recipients whose client
// swallowed the button.
func emailFineprint(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b,
			`<div style="word-wrap:break-word; word-break:break-word; overflow-wrap:anywhere;">%s</div>`,
			htmlEscape(line))
	}
	return fmt.Sprintf(`<tr>
<td style="padding:0 0 %[4]s 0; font-family:%[1]s; font-size:12px; font-weight:400; line-height:18px; color:%[2]s; background-color:%[5]s;">%[3]s</td>
</tr>
`, emailSansStack, emailMutedForeground, b.String(), emailRowGap, emailBackground)
}
