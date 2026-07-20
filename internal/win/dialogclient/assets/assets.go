// Package assets embeds the WebView dialog's HTML/CSS/JS templates. The JS is
// inlined into the HTML at render time because dialogs are served with
// NavigateToString (origin about:blank), where a <script src> cannot resolve.
package assets

import (
	_ "embed"
	"strings"
)

//go:embed dialog.html
var dialogHTML string

//go:embed dialog.js
var dialogJS string

//go:embed progress.html
var progressHTML string

// scriptTag is the external-script placeholder replaced with the inlined JS.
const scriptTag = `<script src="dialog.js"></script>`

// DialogHTML returns the self-contained modal-dialog document with the
// bootstrap script inlined, ready to hand to WebView.SetHtml.
func DialogHTML() string {
	return strings.Replace(dialogHTML, scriptTag, "<script>\n"+dialogJS+"\n</script>", 1)
}

// ProgressHTML returns the self-contained modeless progress document.
func ProgressHTML() string {
	return progressHTML
}
