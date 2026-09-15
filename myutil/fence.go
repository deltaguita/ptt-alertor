package myutil

import "strings"

// Fence marks a block whose alignment depends on a fixed-width font, such as a
// chart's axes. It is written by whatever composes the message, so a channel
// that can show a monospaced block can, and one that cannot simply shows the
// fences.
const Fence = "```"

// FenceToHTML turns fenced blocks into the preformatted markup Telegram
// understands, reporting false when there is nothing to do.
//
// It gives up on an unbalanced pair, which is what a message split across a
// length limit leaves behind: a half-open tag would render the remainder of the
// message as markup rather than as text.
func FenceToHTML(text string) (string, bool) {
	parts := strings.Split(text, Fence)
	if len(parts) < 3 || len(parts)%2 == 0 {
		return "", false
	}
	var sb strings.Builder
	for i, part := range parts {
		if i%2 == 0 {
			sb.WriteString(escapeHTML(part))
			continue
		}
		sb.WriteString("<pre>" + escapeHTML(strings.Trim(part, "\n")) + "</pre>")
	}
	return sb.String(), true
}

func escapeHTML(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}
