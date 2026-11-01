package parse

import "testing"

// A release named only by its number inherits what its listing's name
// says, and keeps what its own name says.
func TestInheritFromListing(t *testing.T) {
	titles := []string{"One Piece"}
	listing := ParseAnchored("One Piece - Digital Colored Comics", ChannelSource, titles)
	r := ParseAnchored("Vol. 001 (Digital)", ChannelSource, titles)
	r.Inherit(listing)
	if !r.Edition.Colored || !r.Claim.Color || r.Claim.ColorEdition {
		t.Fatalf("colorized listing not inherited as a colored variant: %+v %+v", r.Edition, r.Claim)
	}
	group := ParseAnchored("One Piece - !One Piece (Digital) [1r0n]", ChannelSource, titles)
	own := ParseAnchored("One Piece v01 (Digital) (danke-Empire)", ChannelSource, titles)
	own.Inherit(group)
	if own.Group != "danke-Empire" {
		t.Fatalf("the release's own group lost to its listing's: %q", own.Group)
	}
	bare := ParseAnchored("Vol. 01", ChannelSource, titles)
	bare.Inherit(group)
	if bare.Group != "1r0n" || !bare.Quality.Digital {
		t.Fatalf("listing group and quality not inherited: %q %+v", bare.Group, bare.Quality)
	}
}
