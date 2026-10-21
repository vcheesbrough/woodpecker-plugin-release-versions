package plugin

import (
	"fmt"
	"slices"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/gitremote"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/release"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// runTag records that this build was deployed. It runs once the deploy steps
// succeed, whatever smoke tests later find: a broken deploy is still a deploy.
func runTag(cfg types.Config) error {
	if err := requireDeployment(cfg); err != nil {
		return err
	}
	version, err := artifacts.ReadReleaseTag(cfg.OutputDir)
	if err != nil {
		return err
	}
	if _, ok := release.Parse(version, cfg.TagPrefix); !ok {
		return fmt.Errorf(".release-tag %q is not a release version", version)
	}

	remote := remoteFor(cfg)
	// The trailing * keeps the `^{}` line, the only one naming the build's
	// commit; an exact pattern drops it.
	refs, err := remote.ListRefs(release.TagsPrefix+"*", release.BuildsPrefix+version+"*")
	if err != nil {
		return err
	}
	if done, err := checkTag(cfg, refs, version); done || err != nil {
		return err
	}
	// Tags never move, so a version built from another commit must never land
	// here: resolve trusts a tagged commit to ship that version.
	if !slices.Contains(release.BuildsForCommit(refs, cfg.CommitSHA, cfg.TagPrefix), version) {
		return fmt.Errorf("%s is not a build of commit %s; refusing to tag it (let resolve write .release-tag)", version, cfg.CommitSHA)
	}

	if cfg.DryRun {
		fmt.Printf("dry-run: would push tag %s at %s\n", version, cfg.CommitSHA)
		return nil
	}
	obj, err := remote.MakeTag(cfg.OutputDir, gitremote.TagObject{
		Commit:  cfg.CommitSHA,
		Name:    version,
		Message: buildMessage("Release", version, cfg),
	})
	if err != nil {
		return err
	}
	pushErr := remote.Push(cfg.OutputDir, gitremote.Update{Object: obj, Ref: release.TagsPrefix + version})
	if pushErr == nil {
		fmt.Printf("pushed git tag %s\n", version)
		return nil
	}
	// A concurrent deploy of the same commit may have tagged it first.
	refs, err = remote.ListRefs(release.TagsPrefix + "*")
	if err != nil {
		return pushErr
	}
	if done, err := checkTag(cfg, refs, version); done || err != nil {
		return err
	}
	return pushErr
}

// checkTag returns done when the commit already carries this tag, and an error
// when tagging would give the commit a second version or move a tag.
func checkTag(cfg types.Config, refs []types.Ref, version string) (bool, error) {
	existing := release.TagsForCommit(refs, cfg.CommitSHA, cfg.TagPrefix)
	if slices.Contains(existing, version) {
		fmt.Printf("git tag %s already exists at %s\n", version, cfg.CommitSHA)
		return true, nil
	}
	if len(existing) > 0 {
		return false, fmt.Errorf("commit %s already has release tag %s; refusing to add %s", cfg.CommitSHA, existing[0], version)
	}
	if release.TagExists(refs, version) {
		return false, fmt.Errorf("tag %s already exists on another commit; tags never move", version)
	}
	return false, nil
}
