// Package assets embeds static binary assets (images, fonts)
package assets

import _ "embed"

// GorkbotPNG is the Gorkbot mascot image
//
//go:embed gorkbot.png
var GorkbotPNG []byte
