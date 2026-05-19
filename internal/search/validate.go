package search

import (
	"regexp"
	"strings"

	"github.com/dx616b/musicx/internal/prowlarr"
)

var (
	audioPattern = regexp.MustCompile(`(?i)(?:\bflac\b|\bmp3\b|\baac\b|\balac\b|\bape\b|\bwav\b|\bogg\b|\bopus\b|\b320\b|\bkbps\b|\bdiscography\b|\bvinyl\b|\bsingle\b|\bep\b|\balbum\b|\bcd\b|\bbox\s*set\b)`)
	videoPattern = regexp.MustCompile(`(?i)(?:\b(?:480|576|720|1080|2160)p\b|\b4k\b|\bweb[- ]?dl\b|\bwebrip\b|\bbluray\b|\bbrrip\b|\bhdtv\b|\bremux\b|\bx264\b|\bx265\b|\bhevc\b|\bav1\b|\bmkv\b|\bseason\b|\bS\d{1,2}E\d{1,3}\b)`)
)

// IsMusicRelease returns true when a torrent title looks like audio, not video.
func IsMusicRelease(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if videoPattern.MatchString(title) {
		return false
	}
	return audioPattern.MatchString(title)
}

func FilterMusicTorrents(in []*prowlarr.Torrent) []*prowlarr.Torrent {
	out := make([]*prowlarr.Torrent, 0, len(in))
	for _, t := range in {
		if t == nil {
			continue
		}
		if IsMusicRelease(t.Title) {
			out = append(out, t)
		}
	}
	return out
}
