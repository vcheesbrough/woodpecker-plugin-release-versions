package plugin

import (
	"fmt"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/artifacts"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/registry"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// runPublish pushes each locally built image as :<version>. Every check runs
// before the first push, so a refusal publishes nothing.
func runPublish(cfg types.Config, reg Registry, dk Docker) error {
	version, err := artifacts.ReadReleaseTag(cfg.OutputDir)
	if err != nil {
		return err
	}

	refs := make([]string, len(cfg.Images))
	for i, image := range cfg.Images {
		refs[i] = image + ":" + version
	}

	for _, ref := range refs {
		labels, err := dk.Labels(ref)
		if err != nil {
			return err
		}
		if err := registry.CheckLabels(ref, labels, version, cfg.CommitSHA); err != nil {
			return err
		}
	}

	for _, ref := range refs {
		exists, err := reg.Exists(ref)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%s is already published; a version is published once and never overwritten", ref)
		}
	}

	if cfg.DryRun {
		for _, ref := range refs {
			fmt.Printf("dry-run: would push %s\n", ref)
		}
		return nil
	}

	if cfg.RegistryUsername != "" {
		seen := map[string]bool{}
		for _, image := range cfg.Images {
			host, err := registry.Host(image)
			if err != nil {
				return err
			}
			if seen[host] {
				continue
			}
			seen[host] = true
			if err := dk.Login(host, cfg.RegistryUsername, cfg.RegistryPassword); err != nil {
				return err
			}
		}
	}

	for _, ref := range refs {
		if err := dk.Push(ref); err != nil {
			return err
		}
		fmt.Printf("published %s\n", ref)
	}
	return nil
}
