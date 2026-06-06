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
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
	path := filepath.Join(repoRoot, "internal", "version", "version.go")

	major, minor, err := majorminor.Read("go", path)
	if err != nil {
		t.Fatalf("read go version file: %v", err)
	}
	if major != 0 || minor != 1 {
		t.Fatalf("expected release line 0.1.x, got %d.%d", major, minor)
	}
}

func TestValidateAgainstRemote(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	if repo == "" {
		repo = "vcheesbrough/woodpecker-plugin-release-versions"
	}

	cmd := exec.Command("go", "run", ".", "validate")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"CI_COMMIT_SHA="+headSHA(t),
		"CI_REPO="+repo,
		"PLUGIN_GITHUB_TOKEN="+token,
		"PLUGIN_MAJOR_MINOR_SOURCE=go",
		"PLUGIN_MAJOR_MINOR_FILE=internal/version/version.go",
		"CI_COMMIT_BRANCH=main",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out)
	}
}

func TestCandidateImageStarts(t *testing.T) {
	image := os.Getenv("PLUGIN_IMAGE")
	if image == "" {
		t.Skip("PLUGIN_IMAGE not set")
	}

	cmd := exec.Command("docker", "run", "--rm", image, "validate")
	cmd.Env = append(os.Environ(),
		"CI_COMMIT_SHA="+headSHA(t),
		"CI_REPO="+envOr("GITHUB_REPOSITORY", "vcheesbrough/woodpecker-plugin-release-versions"),
		"PLUGIN_GITHUB_TOKEN="+os.Getenv("GITHUB_TOKEN"),
		"PLUGIN_MAJOR_MINOR_SOURCE=go",
		"PLUGIN_MAJOR_MINOR_FILE=internal/version/version.go",
		"CI_COMMIT_BRANCH=main",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("candidate image validate failed: %v\n%s", err, out)
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
