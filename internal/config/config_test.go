package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI_COMMIT_SHA", "abc123")
	t.Setenv("CI_REPO", "owner/repo")
	t.Setenv("PLUGIN_GITHUB_TOKEN", "token")
	t.Setenv("CI_PIPELINE_NUMBER", "7")
}

func TestVersionRequiresMajorMinorSource(t *testing.T) {
	baseEnv(t)
	t.Setenv("PLUGIN_MAJOR_MINOR_SOURCE", "")
	_, err := build(types.ModeVersion)
	if err == nil || !strings.Contains(err.Error(), "major_minor_source") {
		t.Fatalf("expected missing major_minor_source, got %v", err)
	}
}

func TestDefaultsMajorMinorFileForSource(t *testing.T) {
	baseEnv(t)
	for source, want := range map[string]string{"go": "internal/version/version.go", "cargo": "Cargo.toml", "pyproject": "pyproject.toml"} {
		t.Setenv("PLUGIN_MAJOR_MINOR_SOURCE", source)
		t.Setenv("PLUGIN_MAJOR_MINOR_FILE", "")
		cfg, err := build(types.ModeVersion)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MajorMinorFile != want {
			t.Errorf("%s: got %q, want %q", source, cfg.MajorMinorFile, want)
		}
	}
}

func TestParseModeRejectsRemovedModes(t *testing.T) {
	for _, mode := range []string{"compute", "validate"} {
		if _, err := ParseMode(mode); err == nil || !strings.Contains(err.Error(), "removed") {
			t.Errorf("mode %s: expected a removal error, got %v", mode, err)
		}
	}
	if got, _ := ParseMode("push-tag"); got != types.ModeTag {
		t.Errorf("push-tag should alias tag, got %q", got)
	}
}

func TestImagesAcceptCommaListAndJSON(t *testing.T) {
	want := []string{"reg/a", "reg/b"}
	if got := splitList("reg/a, reg/b"); !reflect.DeepEqual(got, want) {
		t.Errorf("comma list: got %v", got)
	}
	if got := splitList(`["reg/a","reg/b"]`); !reflect.DeepEqual(got, want) {
		t.Errorf("json list: got %v", got)
	}
}

func TestEnvironmentsDefaultAndCustom(t *testing.T) {
	baseEnv(t)
	t.Setenv("PLUGIN_IMAGES", "reg/app")
	t.Setenv("CI_PIPELINE_DEPLOY_TARGET", "dev")

	t.Setenv("PLUGIN_ENVIRONMENTS", "")
	cfg, err := build(types.ModeResolve)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environments != nil {
		t.Fatalf("unset environments must stay nil until resolve knows the trunk, got %#v", cfg.Environments)
	}

	t.Setenv("PLUGIN_ENVIRONMENTS", `{"dev":{},"staging":{"branches":["main","release/*"]}}`)
	cfg, err = build(types.ModeResolve)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Environments["staging"].Branches; !reflect.DeepEqual(got, []string{"main", "release/*"}) {
		t.Fatalf("staging branches: got %v", got)
	}
	if _, ok := cfg.Environments["prod"]; ok {
		t.Fatal("custom environments must replace the default, not merge with it")
	}
}

// There is deliberately no ordering option; an unknown key such as the
// dropped require_tag must fail loudly instead of being ignored.
func TestEnvironmentsRejectUnknownKeys(t *testing.T) {
	baseEnv(t)
	t.Setenv("PLUGIN_IMAGES", "reg/app")
	t.Setenv("CI_PIPELINE_DEPLOY_TARGET", "prod")
	t.Setenv("PLUGIN_ENVIRONMENTS", `{"prod":{"branches":["main"],"require_tag":true}}`)
	if _, err := build(types.ModeResolve); err == nil {
		t.Fatal("expected unknown key require_tag to be refused")
	}
}

func TestResolveRequiresImagesAndTarget(t *testing.T) {
	baseEnv(t)
	t.Setenv("PLUGIN_IMAGES", "")
	t.Setenv("CI_PIPELINE_DEPLOY_TARGET", "")
	t.Setenv("PLUGIN_DEPLOY_TARGET", "")
	_, err := build(types.ModeResolve)
	if err == nil || !strings.Contains(err.Error(), "images") || !strings.Contains(err.Error(), "deploy_target") {
		t.Fatalf("expected images and deploy target to be required, got %v", err)
	}
}

func TestGitHubActionsFallbacks(t *testing.T) {
	for _, key := range []string{"CI_COMMIT_SHA", "CI_REPO", "CI_COMMIT_BRANCH", "CI_PIPELINE_NUMBER", "PLUGIN_GITHUB_TOKEN", "PLUGIN_MAJOR_MINOR_SOURCE"} {
		t.Setenv(key, "")
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_SHA", "def456")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")
	t.Setenv("GITHUB_RUN_ID", "991")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	t.Setenv("INPUT_GITHUB_TOKEN", "tok")
	t.Setenv("INPUT_MAJOR_MINOR_SOURCE", "go")

	cfg, err := build(types.ModeVersion)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CommitSHA != "def456" || cfg.Branch != "main" || cfg.PipelineID != "991-2" || cfg.GitHubToken != "tok" {
		t.Fatalf("GitHub fallbacks not applied: %#v", cfg)
	}
	if cfg.GitRemote != "https://github.com/owner/repo.git" {
		t.Fatalf("remote: got %q", cfg.GitRemote)
	}
}

func TestRegistryCredentialsComeInPairs(t *testing.T) {
	baseEnv(t)
	t.Setenv("PLUGIN_IMAGES", "reg/app")
	t.Setenv("PLUGIN_REGISTRY_USERNAME", "ci")
	t.Setenv("PLUGIN_REGISTRY_PASSWORD", "")
	if _, err := build(types.ModePublish); err == nil {
		t.Fatal("expected a username without a password to be refused")
	}
}
