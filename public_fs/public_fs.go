package public_fs

import "embed"

//go:embed novnc
var EmbedFiles embed.FS

//go:embed win_vnc
var EmbedVNC embed.FS
