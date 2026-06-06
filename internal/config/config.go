package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func FromEnv() (types.Config, error) {
	mode, err := parseMode(strings.TrimSpace(os.Getenv("PLUGIN_MODE")))
	if err != nil {
		return types.Config{}, err
	}
	return build(mode)
}

func FromCLI(mode types.Mode, args []string) (types.Config, error) {
	fs := flag.NewFlagSet("release-versions", flag.ContinueOnError)
	_ = fs.Parse(args)
	return build(mode)
}

func build(mode types.Mode) (types.Config, error) {
	commitSHA, err := envRequired("CI_COMMIT_SHA", "PLUGIN_COMMIT_SHA")
	if err != nil {
		return types.Config{}, err
	}
	repo, err := envRequired("CI_REPO", "PLUGIN_REPO")
	if err != nil {
		return types.Config{}, err
	}
	token, err := envRequired("PLUGIN_GITHUB_TOKEN", "GITHUB_TOKEN")
	if err != nil {
		return types.Config{}, err
	}

	source, err := envRequired("PLUGIN_MAJOR_MINOR_SOURCE")
	if err != nil {
		return types.Config{}, err
	}

	majorMinorFile := envFirst("PLUGIN_MAJOR_MINOR_FILE")
	if majorMinorFile == "" {
		majorMinorFile = defaultMajorMinorFile(source)
	}

	outputDir := envFirst("PLUGIN_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "."
	}

	cfg := types.Config{
		Mode:             mode,
		CommitSHA:        commitSHA,
		Repo:             repo,
		GitHubToken:      token,
		Branch:           envFirst("CI_COMMIT_BRANCH"),
		MajorMinorSource: source,
		MajorMinorFile:   majorMinorFile,
		TagPrefix:        envFirst("PLUGIN_TAG_PREFIX"),
		OutputDir:        outputDir,
		DryRun:           envBool("PLUGIN_DRY_RUN"),
		PipelineNumber:   envFirst("CI_PIPELINE_NUMBER"),
	}
	return cfg, nil
}

func defaultMajorMinorFile(source string) string {
	switch source {
	case "cargo":
		return "Cargo.toml"
	case "go":
		return "internal/version/version.go"
	default:
		return ""
	}
}

func parseMode(raw string) (types.Mode, error) {
	switch strings.ToLower(raw) {
	case "compute":
		return types.ModeCompute, nil
	case "push-tag", "push_tag", "pushtag":
		return types.ModePushTag, nil
	case "validate":
		return types.ModeValidate, nil
	case "":
		return "", errors.New("PLUGIN_MODE is required when running as a Woodpecker plugin")
	default:
		return "", fmt.Errorf("unknown mode %q; expected compute, push-tag, or validate", raw)
	}
}

func envRequired(keys ...string) (string, error) {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s is required", strings.Join(keys, " or "))
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
