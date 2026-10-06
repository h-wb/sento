package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/qbittorrent"
)

// A real-shaped torrent: keys before and after "info", nested lists and
// dicts, and binary piece data that contains bencode-looking bytes.
const (
	testInfoDict = "d5:filesld6:lengthi12e4:pathl3:v017:001.jpgeee4:name9:One Piece12:piece lengthi16384e6:pieces20:d3:fooi1ee:xxxxxxxxxe"
	testTorrent  = "d8:announce22:http://tracker.test/an13:announce-listll22:http://tracker.test/anee7:comment4:test4:info" +
		testInfoDict + "8:url-listl15:http://seed.eu/ee"
)

func wantHash() string {
	sum := sha1.Sum([]byte(testInfoDict))
	return hex.EncodeToString(sum[:])
}

func TestTorrentInfoHash(t *testing.T) {
	if got := torrentInfoHash([]byte(testTorrent)); got != wantHash() {
		t.Fatalf("infohash = %q, want %q", got, wantHash())
	}
	for name, data := range map[string]string{
		"empty":            "",
		"not bencode":      "<html>login required</html>",
		"no info":          "d8:announce3:urle",
		"truncated":        testTorrent[:len(testTorrent)-60], // cut inside the info dict
		"info not a dict":  "d4:info3:abce",
		"length overflows": "d4:info99999999999999999999:xe",
		"deep nesting":     "d4:info" + strings.Repeat("l", 10000) + strings.Repeat("e", 10000) + "e",
	} {
		if got := torrentInfoHash([]byte(data)); got != "" {
			t.Errorf("%s: infohash = %q, want none", name, got)
		}
	}
}

func TestMagnetInfoHash(t *testing.T) {
	const hexHash = "c12fe1c06bba254a9dc9f519b335aa7c1367a88a"
	for magnet, want := range map[string]string{
		"magnet:?xt=urn:btih:" + strings.ToUpper(hexHash) + "&dn=One+Piece": hexHash,
		"magnet:?dn=x&xt=urn:btih:YEX6DQDLXISUVHOJ6UM3GNNKPQJWPKEK":         hexHash, // base32
		"magnet:?xt=urn:btmh:1220abcd":                                      "",      // v2 only
		"magnet:?xt=urn:btih:nothex":                                        "",
		"https://example.test/x.torrent":                                    "",
	} {
		if got := magnetInfoHash(magnet); got != want {
			t.Errorf("magnetInfoHash(%q) = %q, want %q", magnet, got, want)
		}
	}
}

// Indexers that publish no infohash used to be untrackable: the queue item
// was keyed by the feed guid, qBittorrent was asked for a torrent with that
// "hash", never had one, and the grab was declared dead ten minutes later
// while it was downloading fine.
func TestGrabKeysTheDownloadByItsRealInfoHash(t *testing.T) {
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testTorrent))
	}))
	defer indexer.Close()
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent) // login and add both succeed
	}))
	defer qbit.Close()

	p := NewTorrent(nil, qbittorrent.New(qbit.URL, "", ""), "sento")
	ctx := context.Background()

	id, err := p.Grab(ctx, Candidate{Title: "One Piece v01", Torrent: &TorrentRef{
		GUID: "https://indexer.test/details/42", DownloadURL: indexer.URL + "/dl/42",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if id != wantHash() {
		t.Fatalf(".torrent grab id = %q, want the infohash %q", id, wantHash())
	}

	id, err = p.Grab(ctx, Candidate{Title: "One Piece v02", Torrent: &TorrentRef{
		GUID: "guid-2", MagnetURL: "magnet:?xt=urn:btih:C12FE1C06BBA254A9DC9F519B335AA7C1367A88A",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if id != "c12fe1c06bba254a9dc9f519b335aa7c1367a88a" {
		t.Fatalf("magnet grab id = %q, want the magnet's infohash", id)
	}

	// The feed's own infohash still wins when present.
	id, err = p.Grab(ctx, Candidate{Title: "One Piece v03", Torrent: &TorrentRef{
		GUID: "guid-3", InfoHash: "ABCDEF", DownloadURL: indexer.URL + "/dl/3",
	}})
	if err != nil || id != "abcdef" {
		t.Fatalf("feed-hash grab id = %q (err %v), want abcdef", id, err)
	}
}
