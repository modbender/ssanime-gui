// Package icon embeds the ssanime-gui system-tray icon.
package icon

import _ "embed"

// Data is the raw PNG bytes of the tray icon (32x32, dark navy + cyan "S" mark).
//
//go:embed icon.png
var Data []byte
