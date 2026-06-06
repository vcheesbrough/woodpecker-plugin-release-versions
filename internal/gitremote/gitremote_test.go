package gitremote

import (
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func TestParseLsRemote(t *testing.T) {
	stdout := "abc111\trefs/tags/1.2.0\ndef222\trefs/tags/1.2.0^{}\nghi333\trefs/tags/1.2.1\n"
	tags := parseLsRemoteForTest(stdout)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	var found *string
	for _, tag := range tags {
		if tag.Name == "1.2.0" {
			found = &tag.SHA
		}
	}
	if found == nil || *found != "def222" {
		t.Fatalf("expected peeled sha def222, got %#v", tags)
	}
}

func TestNextPatchUsesMaxNotCount(t *testing.T) {
	tags := []types.RemoteTag{
		{Name: "1.23.0", SHA: "a"},
		{Name: "1.23.4", SHA: "b"},
	}
	if got := NextPatch(tags, 1, 23, ""); got != 5 {
		t.Fatalf("expected patch 5, got %d", got)
	}
}
