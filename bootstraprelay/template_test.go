package bootstraprelay

import (
	"strings"
	"testing"
)

func TestRenderCloudInitRendersBasicData(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "warn",
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	for _, want := range []string{
		"#cloud-config\n",
		"Image=ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0\n",
		"Environment=RUST_LOG=warn\n",
		"--tls-cert /etc/letsencrypt/live/relay.example.test/fullchain.pem",
		"--tls-key /etc/letsencrypt/live/relay.example.test/privkey.pem\n",
		"path: /etc/containers/systemd/bootstrap.container\n",
		"path: /opt/bootstrap_srv/provision-cert.sh\n",
		"runcmd:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in rendered output:\n%s", want, got)
		}
	}
}

func TestRenderCloudInitAppendsExtraArgs(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "debug",
		ExtraArgs:      []string{"--extra-flag", "value"},
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	want := "/etc/letsencrypt/live/relay.example.test/privkey.pem --extra-flag value\n"
	if !strings.Contains(got, want) {
		t.Errorf("extra args not appended to Exec line; wanted to find %q in:\n%s", want, got)
	}
}

// With no ExtraArgs the range must emit nothing at all, not a trailing space.
func TestRenderCloudInitOmitsExtraArgsWhenEmpty(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "warn",
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	if !strings.Contains(got, "privkey.pem\n") {
		t.Errorf("Exec line does not end cleanly after privkey.pem:\n%s", got)
	}
}

func TestRenderCloudInitSubstitutesHostnameAndContact(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.9.9",
		RustLog:        "warn",
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	for _, want := range []string{
		"Image=ghcr.io/holochain/kitsune2_bootstrap_srv:v0.9.9\n",
		"Environment=RUST_LOG=warn\n",
		`certbot certonly --standalone -d "relay.example.test" --non-interactive --agree-tos -m "ops@example.test";`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in rendered output:\n%s", want, got)
		}
	}
}

// The values certbot receives are rendered into a shell command that runcmd executes
// as root, so they must be quoted even though validation already constrains them.
func TestRenderCloudInitQuotesShellArguments(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "warn",
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	for _, want := range []string{`-d "relay.example.test"`, `-m "ops@example.test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("shell argument is not quoted; wanted %q in:\n%s", want, got)
		}
	}
}
