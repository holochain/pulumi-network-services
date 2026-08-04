package bootstraprelay

import (
	"fmt"

	"github.com/holochain/pulumi-network-services/internal/validate"
)

// validateTemplateData rejects input that would produce malformed cloud-init. Every
// rendered value sits inside a YAML block scalar, so a line break in any of them
// would end the block early and change the meaning of the document.
func validateTemplateData(d templateData) error {
	if err := validate.Hostname("hostname", d.Hostname); err != nil {
		return err
	}
	if err := validate.Email("contactEmail", d.ContactEmail); err != nil {
		return err
	}
	if err := validate.Required("containerImage", d.ContainerImage); err != nil {
		return err
	}
	if err := validate.NoLineBreaks("rustLog", d.RustLog); err != nil {
		return err
	}
	// Empty is valid and means unauthenticated, which is the default.
	if d.AuthHookServer != "" {
		if err := validate.HttpsUrl("authHookServer", d.AuthHookServer); err != nil {
			return err
		}
	}
	for i, arg := range d.ExtraArgs {
		if err := validate.NoLineBreaks(fmt.Sprintf("extraArgs[%d]", i), arg); err != nil {
			return err
		}
	}
	return nil
}
