// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package library

import (
	"regexp"
)

var mediaTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".flac": "audio/flac",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".mov":  "video/quicktime",
}
var itemIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
