package manifests

import "embed"

//go:embed klusterlet/management
//go:embed klusterlet/managed
var KlusterletManifestFiles embed.FS
