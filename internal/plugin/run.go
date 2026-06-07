package plugin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/gitremote"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/majorminor"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/release"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

func Run(cfg types.Config) error {
	switch cfg.Mode {
	case types.ModeCompute:
		return runCompute(cfg)
	case types.ModePushTag:
		return runPushTag(cfg)
	case types.ModeValidate:
		return runValidate(cfg)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}

func runCompute(cfg types.Config) error {
	major, minor, err := majorminor.Read(cfg.MajorMinorSource, cfg.MajorMinorFile)
	if err != nil {
		return err
	}
	line := majorminor.Line(major, minor)

	remoteTags, err := gitremote.ListRemoteTags(cfg)
	if err != nil {
		return err
	}

	reusedTag := gitremote.TagForCommit(remoteTags, cfg.CommitSHA)
	releaseTag := reusedTag
	if releaseTag == "" {
		patch := gitremote.NextPatch(remoteTags, major, minor, cfg.TagPrefix)
		releaseTag = release.FormatReleaseTag(cfg.TagPrefix, major, minor, patch)
	}

	if err := release.ValidateReleaseOnLine(releaseTag, cfg.TagPrefix, major, minor); err != nil {
		return err
	}
	if !release.IsDockerTagSafe(releaseTag) {
		return fmt.Errorf("release tag %q is not docker-tag safe", releaseTag)
	}

	reused := reusedTag != ""
	if err := artifacts.WriteReleaseArtifacts(cfg.OutputDir, releaseTag, line, reused, cfg.DryRun); err != nil {
		return err
	}

	fmt.Printf("RELEASE_TAG=%s\n", releaseTag)
	if reused {
		fmt.Printf("release tag reused for commit %s\n", cfg.CommitSHA)
	} else {
		fmt.Printf("release tag allocated: %s\n", releaseTag)
	}
	return nil
}

func runPushTag(cfg types.Config) error {
	releaseTag, err := artifacts.ReadReleaseTag(cfg.OutputDir)
	if err != nil {
		return err
	}

	remoteTags, err := gitremote.ListRemoteTags(cfg)
	if err != nil {
		return err
	}

	if existing := gitremote.TagForCommit(remoteTags, cfg.CommitSHA); existing != "" {
		if existing == releaseTag {
			fmt.Printf("git tag %s already exists at commit %s\n", releaseTag, cfg.CommitSHA)
			return nil
		}
		return fmt.Errorf(
			"commit %s already has semver tag %s; refusing to create %s",
			cfg.CommitSHA,
			existing,
			releaseTag,
		)
	}

	for _, tag := range remoteTags {
		if tag.Name == releaseTag {
			return fmt.Errorf(
				"tag %s already exists on a different commit (immutability violation)",
				releaseTag,
			)
		}
	}

	if cfg.DryRun {
		fmt.Printf("dry-run: would create annotated tag %s at %s\n", releaseTag, cfg.CommitSHA)
		return nil
	}

	message := fmt.Sprintf(
		"Release %s\n\ncommit: %s\nbranch: %s\npipeline: %s\n",
		releaseTag,
		cfg.CommitSHA,
		cfg.Branch,
		defaultString(cfg.PipelineNumber, "unknown"),
	)

	if err := createAnnotatedTag(cfg, releaseTag, message); err != nil {
		return err
	}
	if err := pushTag(cfg, releaseTag); err != nil {
		return err
	}
	fmt.Printf("pushed git tag %s\n", releaseTag)
	return nil
}

func runValidate(cfg types.Config) error {
	major, minor, err := majorminor.Read(cfg.MajorMinorSource, cfg.MajorMinorFile)
	if err != nil {
		return err
	}
	line := majorminor.Line(major, minor)

	remoteTags, err := gitremote.ListRemoteTags(cfg)
	if err != nil {
		return err
	}

	count := 0
	for _, tag := range remoteTags {
		ver, err := release.ParseTagVersion(tag.Name, cfg.TagPrefix)
		if err != nil || ver.Prerelease() != "" {
			continue
		}
		if ver.Major() == major && ver.Minor() == minor {
			count++
		}
	}
	fmt.Printf("validate ok: manifest line %s, %d existing release tag(s) on remote\n", line, count)

	if existing := gitremote.TagForCommit(remoteTags, cfg.CommitSHA); existing != "" {
		if err := release.ValidateReleaseOnLine(existing, cfg.TagPrefix, major, minor); err != nil {
			return err
		}
		if !release.IsDockerTagSafe(existing) {
			return fmt.Errorf("existing tag %q is not docker-tag safe", existing)
		}
		fmt.Printf("commit already tagged: %s\n", existing)
	}
	return nil
}

func createAnnotatedTag(cfg types.Config, tag, message string) error {
	cmd := gitLocalCommand(cfg,
		"-c", "user.name=woodpecker-plugin-release-versions",
		"-c", "user.email=woodpecker@localhost",
		"tag", "-a", tag, cfg.CommitSHA, "-m", message,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git tag failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func pushTag(cfg types.Config, tag string) error {
	url := fmt.Sprintf("https://x-token:%s@github.com/%s.git", cfg.GitHubToken, cfg.Repo)
	cmd := gitLocalCommand(cfg, "push", url, tag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push tag failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func gitLocalCommand(cfg types.Config, args ...string) *exec.Cmd {
	cmdArgs := append([]string{"-c", "safe.directory=*"}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Dir = cfg.OutputDir
	if abs, err := filepath.Abs(cmd.Dir); err == nil {
		cmd.Dir = abs
	}
	return cmd
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
