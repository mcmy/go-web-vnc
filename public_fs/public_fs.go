package public_fs

import "embed"

//go:embed novnc
var EmbedFiles embed.FS

//go:embed tvnc
var EmbedVNC embed.FS
