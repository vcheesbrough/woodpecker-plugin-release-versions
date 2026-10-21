package release

import (
	"reflect"
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func TestIsDockerTagSafe(t *testing.T) {
	if IsDockerTagSafe("1.2.3+abc") {
		t.Fatal("expected + metadata to be rejected")
	}
	if !IsDockerTagSafe("1.2.3") {
		t.Fatal("expected plain semver to be accepted")
	}
}

func TestParseIsStrict(t *testing.T) {
	for _, name := range []string{"1.2", "v1.2.3", "1.2.3-rc.1", "1.2.3+meta", "01.2.3"} {
		if _, ok := Parse(name, ""); ok {
			t.Errorf("Parse(%q) accepted a non-release name", name)
		}
	}
	if _, ok := Parse("v1.2.3", "v"); !ok {
		t.Error("Parse with prefix v rejected v1.2.3")
	}
}

func TestNextPatchCountsTagsAndBuildsOnTheLine(t *testing.T) {
	refs := []types.Ref{
		{Name: "refs/tags/1.23.4"},
		{Name: "refs/builds/1.23.7"},     // failed build: number still used up
		{Name: "refs/builds/1.24.9"},     // another line
		{Name: "refs/tags/1.23.99-rc.1"}, // not release-shaped
		{Name: "refs/heads/1.23.50"},     // not a tag or build
	}
	if got := NextPatch(refs, 1, 23, ""); got != 8 {
		t.Fatalf("expected 8, got %d", got)
	}
	if got := NextPatch(refs, 2, 0, ""); got != 0 {
		t.Fatalf("expected a new line to start at 0, got %d", got)
	}
}

func TestBuildsForCommitNewestFirst(t *testing.T) {
	refs := []types.Ref{
		{Name: "refs/builds/1.2.9", Peeled: "c1"},
		{Name: "refs/builds/1.2.10", Peeled: "c1"},
		{Name: "refs/builds/1.2.11", Peeled: "other"},
		{Name: "refs/builds/1.1.40", Peeled: "c1"},
	}
	want := []string{"1.2.10", "1.2.9", "1.1.40"}
	if got := BuildsForCommit(refs, "c1", ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTagsForCommitUsesPeeledCommit(t *testing.T) {
	refs := []types.Ref{
		{Name: "refs/tags/1.2.3", SHA: "tagobj", Peeled: "c1"},
		{Name: "refs/tags/1.2.4", SHA: "c1"},
		{Name: "refs/tags/not-a-release", SHA: "c1"},
	}
	want := []string{"1.2.3", "1.2.4"}
	if got := TagsForCommit(refs, "c1", ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPipelineVersionPicksLowestOnLine(t *testing.T) {
	refs := []types.Ref{
		{Name: "refs/build-pipelines/42/1.2.6"},
		{Name: "refs/build-pipelines/42/1.2.5"},
		{Name: "refs/build-pipelines/42/1.3.0"},
		{Name: "refs/build-pipelines/420/1.2.1"},
	}
	if got := PipelineVersion(refs, "42", 1, 2, ""); got != "1.2.5" {
		t.Fatalf("got %q", got)
	}
	if got := PipelineVersion(refs, "43", 1, 2, ""); got != "" {
		t.Fatalf("expected no claim for pipeline 43, got %q", got)
	}
}
