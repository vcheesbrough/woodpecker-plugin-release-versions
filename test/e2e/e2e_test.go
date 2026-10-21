//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/majorminor"
)

func TestReadGoReleaseLineFromRepo(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "version", "version.go")

	major, minor, err := majorminor.Read("go", path)
	if err != nil {
		t.Fatalf("read go version file: %v", err)
	}
	if major != 0 || minor != 2 {
		t.Fatalf("expected release line 0.2.x, got %d.%d", major, minor)
	}
}

// A dry-run version reads the real GitHub remote, including the peeled lines of
// refs outside refs/tags, without claiming anything.
func TestVersionDryRunAgainstRemote(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set")
	}
	cmd := exec.Command("go", "run", ".", "version")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), pluginEnv(t, token)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version dry run failed: %v\n%s", err, out)
	}
}

func TestCandidateImageStarts(t *testing.T) {
	image := os.Getenv("PLUGIN_IMAGE")
	if image == "" {
		t.Skip("PLUGIN_IMAGE not set")
	}
	args := []string{"run", "--rm", "-v", repoRoot(t) + ":/workspace", "-w", "/workspace", "-e", "PLUGIN_MODE=version"}
	for _, kv := range pluginEnv(t, os.Getenv("GITHUB_TOKEN")) {
		args = append(args, "-e", kv)
	}
	out, err := exec.Command("docker", append(args, image)...).CombinedOutput()
	if err != nil {
		t.Fatalf("candidate image version dry run failed: %v\n%s", err, out)
	}
}

func pluginEnv(t *testing.T, token string) []string {
	return []string{
		"CI_COMMIT_SHA=" + headSHA(t),
		"CI_REPO=" + envOr("GITHUB_REPOSITORY", "vcheesbrough/woodpecker-plugin-release-versions"),
		"CI_PIPELINE_NUMBER=e2e",
		"CI_COMMIT_BRANCH=master",
		"PLUGIN_GITHUB_TOKEN=" + token,
		"PLUGIN_MAJOR_MINOR_SOURCE=go",
		"PLUGIN_DRY_RUN=true",
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
}

func headSHA(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return string(out[:len(out)-1])
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
