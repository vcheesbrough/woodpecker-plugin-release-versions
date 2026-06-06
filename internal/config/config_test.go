package config

import (
	"os"
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func TestBuildRequiresMajorMinorSource(t *testing.T) {
	t.Setenv("CI_COMMIT_SHA", "abc123")
	t.Setenv("CI_REPO", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "token")
	os.Unsetenv("PLUGIN_MAJOR_MINOR_SOURCE")

	_, err := build(types.ModeCompute)
	if err == nil {
		t.Fatal("expected error when PLUGIN_MAJOR_MINOR_SOURCE is missing")
	}
}

func TestBuildDefaultsMajorMinorFileForSource(t *testing.T) {
	t.Setenv("CI_COMMIT_SHA", "abc123")
	t.Setenv("CI_REPO", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "token")
	t.Setenv("PLUGIN_MAJOR_MINOR_SOURCE", "go")
	os.Unsetenv("PLUGIN_MAJOR_MINOR_FILE")

	cfg, err := build(types.ModeCompute)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MajorMinorFile != "internal/version/version.go" {
		t.Fatalf("got major minor file %q", cfg.MajorMinorFile)
	}
}
