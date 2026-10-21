package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func pushImage(t *testing.T, ref string, labels map[string]string) {
	t.Helper()
	img, err := random.Image(64, 1)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Config.Labels = labels
	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := name.ParseReference(ref, name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(r, img); err != nil {
		t.Fatal(err)
	}
}

func TestExistsAndLabelsAgainstARealRegistryAPI(t *testing.T) {
	srv := httptest.NewServer(ggcrregistry.New())
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref := host + "/app:1.2.3"
	pushImage(t, ref, map[string]string{LabelVersion: "1.2.3", LabelRevision: "abc"})

	c := New("", "", true)
	if ok, err := c.Exists(ref); err != nil || !ok {
		t.Fatalf("Exists(published) = %v, %v", ok, err)
	}
	if ok, err := c.Exists(host + "/app:9.9.9"); err != nil || ok {
		t.Fatalf("Exists(missing) = %v, %v", ok, err)
	}
	labels, found, err := c.Labels(ref)
	if err != nil || !found {
		t.Fatalf("Labels: found=%v err=%v", found, err)
	}
	if err := CheckLabels(ref, labels, "1.2.3", "abc"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := c.Labels(host + "/other:1.0.0"); err != nil || found {
		t.Fatalf("Labels(unknown repo) found=%v err=%v", found, err)
	}
}

// A registry that errors must never read as "not there": publish would then
// overwrite, and resolve would skip a good build.
func TestRegistryErrorsAreNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ref := strings.TrimPrefix(srv.URL, "http://") + "/app:1.0.0"

	c := New("", "", true)
	if _, err := c.Exists(ref); err == nil {
		t.Fatal("Exists treated a 503 as an answer")
	}
	if _, _, err := c.Labels(ref); err == nil {
		t.Fatal("Labels treated a 503 as an answer")
	}
}

func TestCheckLabelsNamesTheMismatch(t *testing.T) {
	err := CheckLabels("r/app:1.0.0", map[string]string{LabelVersion: "1.0.0", LabelRevision: "other"}, "1.0.0", "abc")
	if err == nil || !strings.Contains(err.Error(), LabelRevision) {
		t.Fatalf("got %v", err)
	}
}

func TestHost(t *testing.T) {
	if got, _ := Host("registry.desync.link/sovereign-config-cli"); got != "registry.desync.link" {
		t.Fatalf("got %q", got)
	}
}
