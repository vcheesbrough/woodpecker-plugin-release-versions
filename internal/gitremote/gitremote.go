package gitremote

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// Remote is the repository the release state lives in. URL may be any git URL
// or a local path, so tests drive real git against a local bare repository.
type Remote struct {
	URL   string
	Token string
}

// command runs git with the token in an HTTP header passed through
// GIT_CONFIG_* environment variables: never in the URL, where git echoes it in
// errors, and never in argv, where any process listing shows it.
func (r Remote) command(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=*"}, args...)...)
	if dir != "" {
		cmd.Dir = dir
		if abs, err := filepath.Abs(dir); err == nil {
			cmd.Dir = abs
		}
	}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if r.Token != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + r.Token))
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic,
		)
	}
	cmd.Env = env
	return cmd
}

func (r Remote) run(dir string, stdin string, args ...string) (string, error) {
	cmd := r.command(dir, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s: %w", args[0], r.redact(strings.TrimSpace(string(out))), err)
	}
	return string(out), nil
}

func (r Remote) redact(s string) string {
	if r.Token == "" {
		return s
	}
	return strings.ReplaceAll(s, r.Token, "***")
}

// ListRefs lists the remote refs matching the given patterns. No `--refs`:
// that flag suppresses the peeled `<ref>^{}` lines, and for a ref pointing at
// an annotated tag object that line is the only one naming the commit.
func (r Remote) ListRefs(patterns ...string) ([]types.Ref, error) {
	out, err := r.run("", "", append([]string{"ls-remote", r.URL}, patterns...)...)
	if err != nil {
		return nil, err
	}
	return ParseLsRemote(out), nil
}

// ParseLsRemote folds each `<ref>^{}` line onto its ref as Peeled.
func ParseLsRemote(stdout string) []types.Ref {
	peeled := map[string]string{}
	direct := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if name, isPeeled := strings.CutSuffix(ref, "^{}"); isPeeled {
			peeled[name] = sha
			continue
		}
		direct[ref] = sha
	}
	out := make([]types.Ref, 0, len(direct))
	for name, sha := range direct {
		out = append(out, types.Ref{Name: name, SHA: sha, Peeled: peeled[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// TagObject is the content of an annotated tag object written without a ref.
type TagObject struct {
	Commit  string
	Name    string
	Message string
}

// MakeTag writes an annotated tag object into the repository at dir and
// returns its id. No local ref is created; the object only reaches the remote
// through Push. dir must already contain the commit.
func (r Remote) MakeTag(dir string, tag TagObject) (string, error) {
	body := fmt.Sprintf(
		"object %s\ntype commit\ntag %s\ntagger woodpecker-plugin-release-versions <woodpecker@localhost> %d +0000\n\n%s",
		tag.Commit, tag.Name, time.Now().Unix(), tag.Message,
	)
	out, err := r.run(dir, body, "mktag")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Update creates Ref at Object on the remote. Updates are create-only: a ref
// that already exists makes the whole push fail.
type Update struct {
	Object string
	Ref    string
}

// Push sends every update in one atomic push, so either all refs are created
// or none are.
func (r Remote) Push(dir string, updates ...Update) error {
	args := []string{"push", "--atomic", "--porcelain"}
	for _, u := range updates {
		// An empty expected value means "this ref must not exist yet".
		args = append(args, "--force-with-lease="+u.Ref+":")
	}
	args = append(args, r.URL)
	for _, u := range updates {
		args = append(args, u.Object+":"+u.Ref)
	}
	_, err := r.run(dir, "", args...)
	return err
}
