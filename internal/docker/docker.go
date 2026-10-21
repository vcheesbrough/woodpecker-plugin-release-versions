package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CLI drives the docker CLI against the agent's daemon, where the build step
// left the image. Logins go to a private config directory, so the agent's own
// docker credentials are never read or changed.
type CLI struct {
	configDir string
}

func NewCLI() (*CLI, error) {
	dir, err := os.MkdirTemp("", "release-versions-docker-")
	if err != nil {
		return nil, err
	}
	return &CLI{configDir: dir}, nil
}

func (c *CLI) Close() error {
	return os.RemoveAll(c.configDir)
}

func (c *CLI) run(stdin string, args ...string) (string, error) {
	cmd := exec.Command("docker", append([]string{"--config", c.configDir}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s failed: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// Labels returns the labels of a locally built image.
func (c *CLI) Labels(image string) (map[string]string, error) {
	out, err := c.run("", "image", "inspect", "--format", "{{json .Config.Labels}}", image)
	if err != nil {
		return nil, fmt.Errorf("image %s is not in the local docker daemon; build it with that tag first: %w", image, err)
	}
	labels := map[string]string{}
	if trimmed := strings.TrimSpace(out); trimmed != "null" {
		if err := json.Unmarshal([]byte(trimmed), &labels); err != nil {
			return nil, fmt.Errorf("parse labels of %s: %w", image, err)
		}
	}
	return labels, nil
}

func (c *CLI) Login(host, username, password string) error {
	_, err := c.run(password, "login", host, "--username", username, "--password-stdin")
	return err
}

func (c *CLI) Push(image string) error {
	_, err := c.run("", "push", image)
	return err
}
