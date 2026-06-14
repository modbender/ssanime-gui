// Package extension implements the goja-based JS extension runtime for
// ssanime-gui. It provides:
//
//   - A VM factory (runtime.go) that compiles JS and injects host bindings
//     (fetch backed by Go http, console.log, setTimeout stub).
//   - A JS→Go provider adapter (adapter.go) that wraps a loaded JS extension
//     and implements source.Provider so the registry treats native + JS providers
//     uniformly.
//   - A Marketplace / repo manager (manager.go) for fetching repo index.json,
//     listing available extensions, installing/enabling/disabling them, and
//     registering them into source.Registry on boot.
//
// ES module loading strategy: goja does not support native ESM import/export
// syntax. Extensions using "export default new class …" are rewritten by a thin
// pre-process step (stripExportDefault) that wraps the payload in an IIFE and
// captures the default export as the global variable "__ssExt", which the
// adapter then addresses. No esbuild dependency is required for this minimal
// transform — it handles the single pattern all known Hayase/ssanime extensions
// use.
package extension

import "encoding/json"

// IndexEntry is one item from a repo's index.json.
type IndexEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Type     string `json:"type"` // "torrent"
	Accuracy string `json:"accuracy"`
	NSFW     bool   `json:"nsfw"`
	Icon     string `json:"icon"`
	Update   string `json:"update"`
	Code     string `json:"code"` // raw JS URL
	// Languages is from Hayase format; we preserve but don't enforce.
	Languages []string `json:"languages"`
	// Options is the per-extension settings schema (Hayase shape: an object of
	// {key: {label,type,value|default,...}}). Kept raw because the schema is
	// extension-specific; resolveSettings extracts the default values.
	Options json.RawMessage `json:"options"`
}

// ExtType is the installed extension type tag stored in the DB.
const ExtTypeTorrent = "torrent"
