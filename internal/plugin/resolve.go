package plugin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/config"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/registry"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/release"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// runResolve finds the build a deployment must ship. A commit with a release
// tag ships that version, so every environment gets the same build in any
// order; otherwise it ships the newest of the commit's published builds.
// .release-tag is written only after every check passes.
func runResolve(cfg types.Config, reg Registry) error {
	if err := requireDeployment(cfg); err != nil {
		return err
	}
	remote := remoteFor(cfg)
	envs, source := cfg.Environments, "environments"
	if envs == nil {
		trunk := cfg.DefaultBranch
		if trunk == "" {
			var err error
			if trunk, err = remote.DefaultBranch(); err != nil {
				return fmt.Errorf("finding the default branch for the default environments: %w", err)
			}
		}
		envs = config.DefaultEnvironments(trunk)
		source = "the default environments (set environments to change)"
	}
	env, ok := envs[cfg.DeployTarget]
	if !ok {
		return fmt.Errorf("deploy target %q is not in %s (have: %s)", cfg.DeployTarget, source, targetNames(envs))
	}
	if !branchAllowed(env.Branches, cfg.Branch) {
		return fmt.Errorf("branch %q may not deploy to %s under %s (allowed: %s)", cfg.Branch, cfg.DeployTarget, source, strings.Join(env.Branches, ", "))
	}

	refs, err := remote.ListRefs(release.TagsPrefix+"*", release.BuildsPrefix+"*")
	if err != nil {
		return err
	}
	if err := requirePeeledBuilds(refs); err != nil {
		return err
	}

	version, err := pickVersion(cfg, reg, refs)
	if err != nil {
		return err
	}
	if err := artifacts.WriteReleaseTag(cfg.OutputDir, version, cfg.GitHubOutput); err != nil {
		return err
	}
	fmt.Printf("RELEASE_TAG=%s\n", version)
	return nil
}

func pickVersion(cfg types.Config, reg Registry, refs []types.Ref) (string, error) {
	tags := release.TagsForCommit(refs, cfg.CommitSHA, cfg.TagPrefix)
	if len(tags) > 1 {
		sort.Strings(tags)
		return "", fmt.Errorf("commit %s has more than one release tag (%s); a commit runs under one version", cfg.CommitSHA, strings.Join(tags, ", "))
	}
	if len(tags) == 1 {
		version := tags[0]
		present, err := allPublished(cfg, reg, version)
		if err != nil {
			return "", err
		}
		if !present {
			return "", fmt.Errorf("commit %s is tagged %s but that build is no longer in the registry (pruned?); it cannot be redeployed", cfg.CommitSHA, version)
		}
		fmt.Printf("commit already deployed as %s; deploying the same build\n", version)
		return version, nil
	}

	builds := release.BuildsForCommit(refs, cfg.CommitSHA, cfg.TagPrefix)
	for _, version := range builds {
		present, err := allPublished(cfg, reg, version)
		if err != nil {
			return "", err
		}
		if present {
			fmt.Printf("deploying build %s\n", version)
			return version, nil
		}
		fmt.Printf("build %s was never published (failed build); skipping\n", version)
	}
	return "", fmt.Errorf("commit %s has no published build; push a fix or re-run the build", cfg.CommitSHA)
}

// allPublished reports whether every image is in the registry at version. An
// image that is there but labelled for another version or commit is an error,
// never a skip: it means the registry and git disagree.
func allPublished(cfg types.Config, reg Registry, version string) (bool, error) {
	for _, image := range cfg.Images {
		ref := image + ":" + version
		labels, found, err := reg.Labels(ref)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		if err := registry.CheckLabels(ref, labels, version, cfg.CommitSHA); err != nil {
			return false, err
		}
	}
	return true, nil
}

// requirePeeledBuilds fails loudly when the remote lists build refs but peels
// none of them. Every build ref points at a tag object, so without the `^{}`
// lines no build can be matched to its commit, and every deploy would
// misleadingly report "no published build".
func requirePeeledBuilds(refs []types.Ref) error {
	builds := 0
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, release.BuildsPrefix) {
			continue
		}
		if ref.Peeled != "" {
			return nil
		}
		builds++
	}
	if builds > 0 {
		return fmt.Errorf("the remote listed %d build refs without peeled ^{} lines, so builds cannot be matched to commits; this git server is not supported", builds)
	}
	return nil
}

func targetNames(envs map[string]types.Environment) string {
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
