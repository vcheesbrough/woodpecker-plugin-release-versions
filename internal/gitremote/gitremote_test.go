package gitremote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestListRemoteTagsResolvesAnnotatedTagsToCommits drives the real `git ls-remote`
// rather than a canned string. runPushTag creates annotated tags, whose ref points
// at a tag object rather than the commit, so the commit is only visible through
// the peeled `^{}` line. TestParseLsRemote passed while ListRemoteTags asked git
// to suppress exactly those lines (`--refs`), so every released commit looked
// untagged: compute allocated a phantom next patch on redeploy, and push-tag
// reported its own tag as an immutability violation.
func TestListRemoteTagsResolvesAnnotatedTagsToCommits(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "remote.git")
	work := filepath.Join(dir, "work")

	git := func(cwd string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	git(dir, "init", "-q", "--bare", bare)
	git(dir, "init", "-q", work)
	git(work, "commit", "-q", "--allow-empty", "-m", "released commit")
	commit := git(work, "rev-parse", "HEAD")
	git(work, "tag", "-a", "1.2.0", "-m", "Release 1.2.0")
	git(work, "push", "-q", bare, "refs/tags/1.2.0")

	tags, err := listRemoteTags(bare)
	if err != nil {
		t.Fatalf("listRemoteTags: %v", err)
	}
	if got := TagForCommit(tags, commit); got != "1.2.0" {
		t.Fatalf("expected annotated tag 1.2.0 to resolve to commit %s, got %q from %#v", commit, got, tags)
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
