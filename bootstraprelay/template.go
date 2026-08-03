package bootstraprelay

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bootstraprelay.yaml.tmpl
var cloudInitTemplateSource string

var cloudInitTemplate = template.Must(
	template.New("bootstraprelay-cloud-init").Parse(cloudInitTemplateSource),
)

// templateData is the fully resolved input to the cloud-init template. Every
// field is a plain value: Pulumi Inputs are resolved before this struct is built,
// which is what keeps rendering testable without a Pulumi context.
type templateData struct {
	Hostname       string
	ContactEmail   string
	ContainerImage string
	RustLog        string
	ExtraArgs      []string
}

// renderCloudInit produces the cloud-init user data for a bootstrap/relay host.
func renderCloudInit(data templateData) (string, error) {
	var buf bytes.Buffer
	if err := cloudInitTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render cloud-init: %w", err)
	}
	return buf.String(), nil
}
