// Package ui embeds the browser configuration page served as a CPA plugin
// resource.
package ui

import _ "embed"

//go:embed index.html
var IndexHTML []byte
