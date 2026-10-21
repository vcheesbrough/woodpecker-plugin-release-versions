package plugin

import (
	"errors"
	"fmt"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/gitremote"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/majorminor"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/release"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

const maxClaimAttempts = 20

// runVersion claims the next number on the manifest's line by creating
// refs/builds/<version> on the remote. The remote refuses a ref that already
// exists, so two builds can never hold the same version; the loser re-reads
// and claims the next number.
func runVersion(cfg types.Config) error {
	if cfg.Event == "deployment" {
		return errors.New("mode version runs in build pipelines; a deployment finds its build with mode resolve")
	}
	major, minor, err := majorminor.Read(cfg.MajorMinorSource, cfg.MajorMinorFile)
	if err != nil {
		return err
	}
	remote := remoteFor(cfg)
	patterns := []string{
		release.TagsPrefix + "*",
		release.BuildsPrefix + "*",
		release.PipelinesPrefix + cfg.PipelineID + "/*",
	}
	refs, err := remote.ListRefs(patterns...)
	if err != nil {
		return err
	}

	for attempt := 1; attempt <= maxClaimAttempts; attempt++ {
		// Another workflow of this pipeline already claimed: use its number.
		if claimed := release.PipelineVersion(refs, cfg.PipelineID, major, minor, cfg.TagPrefix); claimed != "" {
			fmt.Printf("pipeline %s already claimed %s\n", cfg.PipelineID, claimed)
			return finishVersion(cfg, claimed)
		}

		candidate := release.FormatReleaseTag(cfg.TagPrefix, major, minor, release.NextPatch(refs, major, minor, cfg.TagPrefix))
		if !release.IsDockerTagSafe(candidate) {
			return fmt.Errorf("version %q is not a valid docker tag", candidate)
		}
		if cfg.DryRun {
			fmt.Printf("dry-run: would claim %s for commit %s\n", candidate, cfg.CommitSHA)
			return nil
		}

		claimErr := claim(cfg, remote, candidate)
		if claimErr == nil {
			// Re-read so racing workflows of one pipeline settle on one number.
			after, err := remote.ListRefs(release.PipelinesPrefix + cfg.PipelineID + "/*")
			if err != nil {
				return err
			}
			if settled := release.PipelineVersion(after, cfg.PipelineID, major, minor, cfg.TagPrefix); settled != "" {
				candidate = settled
			}
			fmt.Printf("claimed %s\n", candidate)
			return finishVersion(cfg, candidate)
		}

		refs, err = remote.ListRefs(patterns...)
		if err != nil {
			return fmt.Errorf("claim %s: %w (and re-reading the remote failed: %v)", candidate, claimErr, err)
		}
		if !release.BuildExists(refs, candidate) && release.PipelineVersion(refs, cfg.PipelineID, major, minor, cfg.TagPrefix) == "" {
			return fmt.Errorf("claim %s: %w", candidate, claimErr)
		}
		fmt.Printf("%s was claimed by another build; retrying (attempt %d)\n", candidate, attempt)
	}
	return fmt.Errorf("could not claim a version after %d attempts", maxClaimAttempts)
}

// claim pushes refs/builds/<version> and this pipeline's marker in one atomic,
// create-only push. Both point at a small annotated tag object naming the
// commit, branch and pipeline rather than at the commit itself: git skips the
// create-only check when a push would leave a ref at the value it already
// has, so two builds of one commit pushing the bare commit could both
// "create" the same ref.
func claim(cfg types.Config, remote gitremote.Remote, version string) error {
	obj, err := remote.MakeTag(cfg.OutputDir, gitremote.TagObject{
		Commit:  cfg.CommitSHA,
		Name:    version,
		Message: buildMessage("Build", version, cfg),
	})
	if err != nil {
		return err
	}
	return remote.Push(cfg.OutputDir,
		gitremote.Update{Object: obj, Ref: release.BuildsPrefix + version},
		gitremote.Update{Object: obj, Ref: release.PipelineRef(cfg.PipelineID, version)},
	)
}

func finishVersion(cfg types.Config, version string) error {
	if err := artifacts.WriteReleaseTag(cfg.OutputDir, version, cfg.GitHubOutput); err != nil {
		return err
	}
	fmt.Printf("RELEASE_TAG=%s\n", version)
	return nil
}
