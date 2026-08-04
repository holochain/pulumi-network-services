package authserver

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/holochain/pulumi-network-services/internal/validate"
)

//go:embed templates/authserver.yaml.tmpl
var cloudInitTemplateSource string

var cloudInitTemplate = template.Must(
	template.New("authserver-cloud-init").Parse(cloudInitTemplateSource),
)

// templateData is the fully resolved input to the cloud-init template. Every field
// is a plain value: Pulumi Inputs are resolved before this struct is built, which
// is what keeps rendering testable without a Pulumi context.
type templateData struct {
	Hostname           string
	ContactEmail       string
	ContainerImage     string
	GithubClientId     string
	GithubClientSecret string
	GithubOrg          string
	GithubTeam         string
	SessionSecret      string
	ApiTokens          string
	RedisUrl           string
	MaxPendingRequests int
}

// renderCloudInit produces the cloud-init user data for an auth server host.
func renderCloudInit(data templateData) (string, error) {
	if err := validateTemplateData(data); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := cloudInitTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render cloud-init: %w", err)
	}
	return buf.String(), nil
}

// validateTemplateData rejects input that would produce malformed cloud-init.
//
// Most of these values land in an environment file rather than a shell command, so
// a line break is the dangerous character: it would end the YAML block scalar early
// and, in the env file, silently truncate every setting that followed.
func validateTemplateData(d templateData) error {
	if err := validate.Hostname("hostname", d.Hostname); err != nil {
		return err
	}
	if err := validate.Email("contactEmail", d.ContactEmail); err != nil {
		return err
	}
	for _, f := range []struct {
		name  string
		value string
	}{
		{"containerImage", d.ContainerImage},
		{"githubClientId", d.GithubClientId},
		{"githubClientSecret", d.GithubClientSecret},
		{"githubOrg", d.GithubOrg},
		{"githubTeam", d.GithubTeam},
		{"sessionSecret", d.SessionSecret},
		{"apiTokens", d.ApiTokens},
		{"redisUrl", d.RedisUrl},
	} {
		if err := validate.Required(f.name, f.value); err != nil {
			return err
		}
	}

	// The server refuses to start below this, so catching it here turns a droplet
	// that boots and never serves into a failure at preview.
	if len(d.SessionSecret) < minSessionSecretBytes {
		return fmt.Errorf(
			"sessionSecret must be at least %d bytes, got %d",
			minSessionSecretBytes, len(d.SessionSecret))
	}
	if d.MaxPendingRequests <= 0 {
		return fmt.Errorf("maxPendingRequests must be positive, got %d", d.MaxPendingRequests)
	}

	// An equals sign is legal in a value, but a leading one, or a name-like prefix
	// in a secret, usually means the caller passed "KEY=value" by mistake.
	if strings.HasPrefix(d.SessionSecret, "=") || strings.HasPrefix(d.ApiTokens, "=") {
		return fmt.Errorf("secret values must not begin with '=': looks like a KEY=value pair was passed whole")
	}
	return nil
}
