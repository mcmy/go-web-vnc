package public_fs

import "embed"

//go:embed novnc
var EmbedFiles embed.FS

//go:embed tight_vnc
var EmbedVNC embed.FS
