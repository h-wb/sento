package provider

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
)

// torrentInfoHash returns the v1 infohash of a .torrent file — the SHA-1 of
// its bencoded "info" dictionary, exactly as it appears in the file — or ""
// when data is not a well-formed torrent.
func torrentInfoHash(data []byte) string {
	if len(data) == 0 || data[0] != 'd' {
		return ""
	}
	pos := 1
	for pos < len(data) && data[pos] != 'e' {
		keyStart, keyEnd, next, ok := bencodeString(data, pos)
		if !ok {
			return ""
		}
		end, ok := bencodeSkip(data, next, 0)
		if !ok {
			return ""
		}
		if string(data[keyStart:keyEnd]) == "info" {
			if data[next] != 'd' {
				return ""
			}
			sum := sha1.Sum(data[next:end])
			return hex.EncodeToString(sum[:])
		}
		pos = end
	}
	return ""
}

// bencodeString reads the string at pos ("<len>:<bytes>") and returns the
// bounds of its bytes plus the offset just past it.
func bencodeString(data []byte, pos int) (start, end, next int, ok bool) {
	colon := pos
	for colon < len(data) && data[colon] != ':' {
		if data[colon] < '0' || data[colon] > '9' {
			return 0, 0, 0, false
		}
		colon++
	}
	if colon == pos || colon >= len(data) {
		return 0, 0, 0, false
	}
	n, err := strconv.Atoi(string(data[pos:colon]))
	if err != nil || n < 0 || n > len(data)-colon-1 {
		return 0, 0, 0, false
	}
	start = colon + 1
	return start, start + n, start + n, true
}

// maxBencodeDepth bounds nesting so a hostile file cannot exhaust the stack.
const maxBencodeDepth = 64

// bencodeSkip returns the offset just past the value starting at pos.
func bencodeSkip(data []byte, pos, depth int) (int, bool) {
	if pos >= len(data) || depth > maxBencodeDepth {
		return 0, false
	}
	switch c := data[pos]; {
	case c == 'i':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			end++
		}
		if end >= len(data) {
			return 0, false
		}
		return end + 1, true
	case c == 'l' || c == 'd':
		pos++
		for pos < len(data) && data[pos] != 'e' {
			if c == 'd' {
				_, _, next, ok := bencodeString(data, pos)
				if !ok {
					return 0, false
				}
				pos = next
			}
			next, ok := bencodeSkip(data, pos, depth+1)
			if !ok {
				return 0, false
			}
			pos = next
		}
		if pos >= len(data) {
			return 0, false
		}
		return pos + 1, true
	case c >= '0' && c <= '9':
		_, _, next, ok := bencodeString(data, pos)
		return next, ok
	}
	return 0, false
}

// magnetInfoHash extracts the v1 infohash from a magnet link's
// xt=urn:btih: parameter (40 hex chars, or 32 base32 chars), lowercased
// hex; "" when the link carries none.
func magnetInfoHash(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		hash, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:")
		if !ok {
			continue
		}
		switch len(hash) {
		case 40:
			if _, err := hex.DecodeString(hash); err == nil {
				return hash
			}
		case 32:
			if raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash)); err == nil {
				return hex.EncodeToString(raw)
			}
		}
	}
	return ""
}
