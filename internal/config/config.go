package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// DefaultEnvironments applies when a repo sets no `environments`: dev from any
// branch, prod from main.
func DefaultEnvironments() map[string]types.Environment {
	return map[string]types.Environment{
		"dev":  {},
		"prod": {Branches: []string{"main"}},
	}
}

// FromEnv reads Woodpecker plugin settings (PLUGIN_*), falling back to GitHub
// Actions inputs (INPUT_*) and GitHub's own variables when GITHUB_ACTIONS=true.
func FromEnv() (types.Config, error) {
	mode, err := ParseMode(setting("MODE"))
	if err != nil {
		return types.Config{}, err
	}
	return build(mode)
}

func FromCLI(mode types.Mode) (types.Config, error) {
	return build(mode)
}

func build(mode types.Mode) (types.Config, error) {
	gha := strings.EqualFold(os.Getenv("GITHUB_ACTIONS"), "true")
	ci := func(woodpecker, github string) string {
		if v := envFirst(woodpecker); v != "" {
			return v
		}
		if gha {
			return envFirst(github)
		}
		return ""
	}

	cfg := types.Config{
		Mode:             mode,
		CommitSHA:        firstNonEmpty(ci("CI_COMMIT_SHA", "GITHUB_SHA"), setting("COMMIT_SHA")),
		Repo:             firstNonEmpty(ci("CI_REPO", "GITHUB_REPOSITORY"), setting("REPO")),
		GitHubToken:      firstNonEmpty(setting("GITHUB_TOKEN"), envFirst("GITHUB_TOKEN")),
		GitRemote:        setting("GIT_REMOTE"),
		Branch:           ci("CI_COMMIT_BRANCH", "GITHUB_REF_NAME"),
		Event:            envFirst("CI_PIPELINE_EVENT"),
		DeployTarget:     firstNonEmpty(setting("DEPLOY_TARGET"), envFirst("CI_PIPELINE_DEPLOY_TARGET")),
		PipelineID:       ci("CI_PIPELINE_NUMBER", "GITHUB_RUN_ID"),
		MajorMinorSource: setting("MAJOR_MINOR_SOURCE"),
		MajorMinorFile:   setting("MAJOR_MINOR_FILE"),
		TagPrefix:        setting("TAG_PREFIX"),
		OutputDir:        firstNonEmpty(setting("OUTPUT_DIR"), "."),
		DryRun:           isTrue(setting("DRY_RUN")),
		Images:           splitList(setting("IMAGES")),
		RegistryUsername: setting("REGISTRY_USERNAME"),
		RegistryPassword: setting("REGISTRY_PASSWORD"),
		RegistryInsecure: isTrue(setting("REGISTRY_INSECURE")),
		GitHubOutput:     envFirst("GITHUB_OUTPUT"),
	}
	if cfg.MajorMinorFile == "" {
		cfg.MajorMinorFile = defaultMajorMinorFile(cfg.MajorMinorSource)
	}
	if cfg.GitRemote == "" && cfg.Repo != "" {
		cfg.GitRemote = fmt.Sprintf("https://github.com/%s.git", cfg.Repo)
	}

	envs, err := parseEnvironments(setting("ENVIRONMENTS"))
	if err != nil {
		return types.Config{}, err
	}
	cfg.Environments = envs

	return cfg, validate(cfg)
}

func validate(cfg types.Config) error {
	var missing []string
	need := func(ok bool, name string) {
		if !ok {
			missing = append(missing, name)
		}
	}
	need(cfg.CommitSHA != "", "CI_COMMIT_SHA")

	switch cfg.Mode {
	case types.ModeVersion:
		need(cfg.GitRemote != "", "CI_REPO")
		need(cfg.GitHubToken != "", "github_token")
		need(cfg.MajorMinorSource != "", "major_minor_source")
		need(cfg.PipelineID != "", "CI_PIPELINE_NUMBER")
	case types.ModePublish:
		need(len(cfg.Images) > 0, "images")
	case types.ModeResolve:
		need(cfg.GitRemote != "", "CI_REPO")
		need(cfg.GitHubToken != "", "github_token")
		need(len(cfg.Images) > 0, "images")
		need(cfg.DeployTarget != "", "CI_PIPELINE_DEPLOY_TARGET or deploy_target")
	case types.ModeTag:
		need(cfg.GitRemote != "", "CI_REPO")
		need(cfg.GitHubToken != "", "github_token")
	}
	if len(missing) > 0 {
		return fmt.Errorf("mode %s: missing %s", cfg.Mode, strings.Join(missing, ", "))
	}
	if (cfg.RegistryUsername == "") != (cfg.RegistryPassword == "") {
		return errors.New("registry_username and registry_password must be set together")
	}
	return nil
}

func ParseMode(raw string) (types.Mode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "version":
		return types.ModeVersion, nil
	case "publish":
		return types.ModePublish, nil
	case "resolve":
		return types.ModeResolve, nil
	case "tag", "push-tag", "push_tag", "pushtag":
		return types.ModeTag, nil
	case "compute", "validate":
		return "", fmt.Errorf("mode %q was removed in 0.2.0; use version (build) or resolve (deploy)", raw)
	case "":
		return "", errors.New("PLUGIN_MODE is required when running as a plugin")
	default:
		return "", fmt.Errorf("unknown mode %q; expected version, publish, resolve, or tag", raw)
	}
}

// parseEnvironments reads the `environments` setting. Woodpecker hands a YAML
// map to the plugin as JSON; unknown keys are refused so a typo cannot
// silently drop a branch rule.
func parseEnvironments(raw string) (map[string]types.Environment, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultEnvironments(), nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var envs map[string]types.Environment
	if err := dec.Decode(&envs); err != nil {
		return nil, fmt.Errorf("environments: %w", err)
	}
	if len(envs) == 0 {
		return nil, errors.New("environments: at least one target is required")
	}
	return envs, nil
}

// splitList accepts Woodpecker's comma-joined list or a JSON array.
func splitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if strings.HasPrefix(raw, "[") && json.Unmarshal([]byte(raw), &out) == nil {
		return out
	}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func defaultMajorMinorFile(source string) string {
	switch source {
	case "cargo":
		return "Cargo.toml"
	case "go":
		return "internal/version/version.go"
	case "pyproject":
		return "pyproject.toml"
	default:
		return ""
	}
}

func setting(name string) string {
	return envFirst("PLUGIN_"+name, "INPUT_"+name)
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func isTrue(raw string) bool {
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
