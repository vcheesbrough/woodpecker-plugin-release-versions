package plugin

import (
	"fmt"
	"path"
	"strings"

	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/docker"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/gitremote"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/registry"
	"github.com/vcheesbrough/woodpecker-plugin-release-versions/internal/types"
)

// Registry is the part of the registry API the modes need.
type Registry interface {
	Exists(ref string) (bool, error)
	Labels(ref string) (labels map[string]string, found bool, err error)
}

// Docker is the local daemon publish pushes from.
type Docker interface {
	Labels(image string) (map[string]string, error)
	Login(host, username, password string) error
	Push(image string) error
}

type Deps struct {
	Registry Registry
	Docker   Docker
}

func Run(cfg types.Config) error {
	deps := Deps{Registry: registry.New(cfg.RegistryUsername, cfg.RegistryPassword, cfg.RegistryInsecure)}
	if cfg.Mode == types.ModePublish {
		cli, err := docker.NewCLI()
		if err != nil {
			return err
		}
		defer cli.Close()
		deps.Docker = cli
	}
	return RunWith(cfg, deps)
}

func RunWith(cfg types.Config, deps Deps) error {
	switch cfg.Mode {
	case types.ModeVersion:
		return runVersion(cfg)
	case types.ModePublish:
		return runPublish(cfg, deps.Registry, deps.Docker)
	case types.ModeResolve:
		return runResolve(cfg, deps.Registry)
	case types.ModeTag:
		return runTag(cfg)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}

func remoteFor(cfg types.Config) gitremote.Remote {
	return gitremote.Remote{URL: cfg.GitRemote, Token: cfg.GitHubToken}
}

// requireDeployment holds deploy-only modes to deployment pipelines. Outside
// Woodpecker the event is unknown and the check is left to the workflow.
func requireDeployment(cfg types.Config) error {
	if cfg.Event != "" && cfg.Event != "deployment" {
		return fmt.Errorf("mode %s runs in deployment pipelines, not on %q events", cfg.Mode, cfg.Event)
	}
	return nil
}

func branchAllowed(patterns []string, branch string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if ok, _ := path.Match(p, branch); ok {
			return true
		}
	}
	return false
}

func buildMessage(kind, version string, cfg types.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\ncommit: %s\nbranch: %s\npipeline: %s\n", kind, version, cfg.CommitSHA, orUnknown(cfg.Branch), orUnknown(cfg.PipelineID))
	if cfg.DeployTarget != "" {
		fmt.Fprintf(&b, "target: %s\n", cfg.DeployTarget)
	}
	return b.String()
}

func orUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
