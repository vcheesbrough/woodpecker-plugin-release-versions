package registry

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

const (
	LabelVersion  = "org.opencontainers.image.version"
	LabelRevision = "org.opencontainers.image.revision"
)

// Client reads images through the registry API, so resolve needs no docker
// socket.
type Client struct {
	auth     authn.Authenticator
	insecure bool
}

func New(username, password string, insecure bool) *Client {
	var auth authn.Authenticator = authn.Anonymous
	if username != "" {
		auth = &authn.Basic{Username: username, Password: password}
	}
	return &Client{auth: auth, insecure: insecure}
}

func (c *Client) parse(ref string) (name.Reference, error) {
	var opts []name.Option
	if c.insecure {
		opts = append(opts, name.Insecure)
	}
	return name.ParseReference(ref, append(opts, name.StrictValidation)...)
}

// Exists reports whether ref is in the registry. Any answer other than found
// or not-found is an error: a registry that cannot answer must not read as
// "free to publish" or "not built".
func (c *Client) Exists(ref string) (bool, error) {
	r, err := c.parse(ref)
	if err != nil {
		return false, err
	}
	if _, err := remote.Head(r, remote.WithAuth(c.auth)); err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("registry cannot answer for %s: %w", ref, err)
	}
	return true, nil
}

// Labels returns the image config labels of ref (the linux/amd64 image when ref
// is a multi-platform index). found is false when ref is not in the registry.
func (c *Client) Labels(ref string) (labels map[string]string, found bool, err error) {
	r, err := c.parse(ref)
	if err != nil {
		return nil, false, err
	}
	img, err := remote.Image(r, remote.WithAuth(c.auth))
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("registry cannot answer for %s: %w", ref, err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, false, fmt.Errorf("read config of %s: %w", ref, err)
	}
	return cfg.Config.Labels, true, nil
}

// Host is the registry an image reference names, for docker login.
func Host(image string) (string, error) {
	r, err := name.ParseReference(image)
	if err != nil {
		return "", err
	}
	return r.Context().RegistryStr(), nil
}

func isNotFound(err error) bool {
	var terr *transport.Error
	if !errors.As(err, &terr) {
		return false
	}
	if terr.StatusCode == http.StatusNotFound {
		return true
	}
	for _, d := range terr.Errors {
		if d.Code == transport.ManifestUnknownErrorCode || d.Code == transport.NameUnknownErrorCode {
			return true
		}
	}
	return false
}

// CheckLabels confirms an image says it is this version built from this commit.
func CheckLabels(ref string, labels map[string]string, version, commit string) error {
	if got := labels[LabelVersion]; got != version {
		return fmt.Errorf("%s: label %s is %q, want %q", ref, LabelVersion, got, version)
	}
	if got := labels[LabelRevision]; got != commit {
		return fmt.Errorf("%s: label %s is %q, want commit %s", ref, LabelRevision, got, commit)
	}
	return nil
}
