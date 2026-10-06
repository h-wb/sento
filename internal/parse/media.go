package parse

import "regexp"

// Release-name tokens that only other media carry. A general tracker
// answers "Hunter x Hunter" with the anime, the films and the games, and
// none of those names say "anime": the codec, source and container do.
// Every token here is one a manga release never uses — "WEB", "Digital"
// and bare resolutions like 1920 are deliberately absent.
var (
	reVideoToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(` +
		`[xh][. ]?26[45]|hevc|avc|av1|xvid|divx|` + // codecs
		`blu-?ray|b[dr]rip|dvd-?rip|web-?rip|web-?dl|hdtv|hdrip|remux|hdlight|4klight|` + // sources
		`(?:480|576|720|1080|2160)[pi]|` + // video resolutions
		`s\d{1,2}e\d{1,3}|` + // S01E02
		`e?ac3|dts|truehd|ddp|vostfr|subfrench|` + // audio tracks and subtitle tags
		`mkv|mp4|avi|fhd` + // containers, "full HD"
		`)(?:[^a-z0-9]|$)`)
	reAudioToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(m4b|mp3|flac|audiobook|\d+ ?kbps|\d+(?:\.\d+)? ?khz)(?:[^a-z0-9]|$)`)
	// Three-part versions ("v1.4.1") and console/installer formats. "REPACK"
	// is absent: scene book and video releases use it for a re-release.
	reSoftwareToken = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(v\d+\.\d+\.\d+(?:\.\d+)*|nsp|xci|nsw|dlcs?|denuvoless|clonecd|fitgirl|elamigos)(?:[^a-z0-9]|$)`)
)

// detectMedia overrides the content type when the name carries another
// medium's tokens. It runs last: these tokens outrank any book vocabulary
// ("Light Novel ... 1080p BluRay" is the adaptation, not the novel).
func detectMedia(cleaned string, res *Result) {
	for _, m := range []struct {
		re *regexp.Regexp
		ct ContentType
	}{
		{reVideoToken, ContentVideo},
		{reSoftwareToken, ContentSoftware},
		{reAudioToken, ContentAudiobook},
	} {
		if hit := m.re.FindStringSubmatch(cleaned); hit != nil {
			res.ContentType = m.ct
			res.step("content.media", hit[1])
			return
		}
	}
}
