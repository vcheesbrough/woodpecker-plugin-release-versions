// Package testgit builds throwaway git repositories for tests: a bare remote
// and a work clone with one commit, so code under test drives real git.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Repo struct {
	t      *testing.T
	Remote string
	Work   string
	Commit string
}

// New makes a bare remote and a work tree holding one commit pushed to main.
func New(t *testing.T) *Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	r := &Repo{t: t, Remote: filepath.Join(dir, "remote.git"), Work: filepath.Join(dir, "work")}
	r.Git(dir, "init", "-q", "--bare", r.Remote)
	r.Git(dir, "init", "-q", "-b", "main", r.Work)
	r.Commit = r.NewCommit("first")
	r.Git(r.Work, "push", "-q", r.Remote, "main")
	return r
}

// NewCommit adds an empty commit to the work tree and returns its id.
func (r *Repo) NewCommit(message string) string {
	r.t.Helper()
	r.Git(r.Work, "commit", "-q", "--allow-empty", "-m", message)
	return r.Git(r.Work, "rev-parse", "HEAD")
}

// Git runs git in dir with a fixed identity and fails the test on error.
func (r *Repo) Git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// RemoteRefs lists every ref on the remote as name -> object id.
func (r *Repo) RemoteRefs() map[string]string {
	r.t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(r.Git(r.Remote, "for-each-ref", "--format=%(refname) %(objectname)"), "\n") {
		if name, sha, ok := strings.Cut(line, " "); ok {
			out[name] = sha
		}
	}
	return out
}

// AnnotatedTag pushes an annotated release tag at commit, as an older release
// scheme or a previous deploy would have.
func (r *Repo) AnnotatedTag(name, commit string) {
	r.t.Helper()
	r.Git(r.Work, "tag", "-a", name, commit, "-m", "Release "+name)
	r.Git(r.Work, "push", "-q", r.Remote, "refs/tags/"+name)
	r.Git(r.Work, "tag", "-d", name)
}
