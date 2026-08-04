package bootstraprelay

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func validAuthenticatedArgs() *AuthenticatedArgs {
	return &AuthenticatedArgs{
		RelayHostname:      pulumi.String("relay.example.test"),
		AuthHostname:       pulumi.String("auth.example.test"),
		ContactEmail:       pulumi.String("ops@example.test"),
		GithubClientId:     pulumi.String("Iv1.0123456789abcdef"),
		GithubClientSecret: pulumi.String("0123456789abcdef0123456789abcdef01234567"),
		GithubOrg:          pulumi.String("holochain"),
		GithubTeam:         pulumi.String("auth"),
		SessionSecret:      pulumi.String(strings.Repeat("s", 64)),
		ApiTokens:          pulumi.String("token1,token2"),
	}
}

// The point of the paired component: the relay is pointed at the auth server this
// component created, so the two cannot drift apart or be wired to nothing.
func TestNewAuthenticatedWiresTheRelayToItsAuthServer(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		pair, err := NewAuthenticated(ctx, "net", validAuthenticatedArgs())
		if err != nil {
			return err
		}

		done := make(chan struct{})
		pulumi.All(pair.Relay.Droplet.UserData, pair.AuthUrl).ApplyT(func(o []interface{}) error {
			defer close(done)

			userData, ok := o[0].(*string)
			if !ok || userData == nil {
				t.Errorf("relay droplet has no user data")
				return nil
			}
			authUrl := o[1].(string)

			want := "--authentication-hook-server " + authUrl
			if !strings.Contains(*userData, want) {
				t.Errorf("relay is not wired to the auth server; wanted %q in:\n%s", want, *userData)
			}
			if authUrl != "https://auth.example.test" {
				t.Errorf("authUrl = %q", authUrl)
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// Q6: one region for the pair. Splitting them would put every authentication
// round-trip across the public internet.
func TestNewAuthenticatedPutsBothHostsInOneRegion(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		args := validAuthenticatedArgs()
		args.Region = pulumi.String("nyc1")
		pair, err := NewAuthenticated(ctx, "net", args)
		if err != nil {
			return err
		}

		done := make(chan struct{})
		pulumi.All(
			pair.Relay.Droplet.Region,
			pair.Auth.Droplet.Region,
			pair.Auth.Database.Region,
		).ApplyT(func(o []interface{}) error {
			defer close(done)
			for i, what := range []string{"relay droplet", "auth droplet", "database"} {
				if got := o[i].(string); got != "nyc1" {
					t.Errorf("%s region = %q, want nyc1", what, got)
				}
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

func TestNewAuthenticatedExportsBothHosts(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		pair, err := NewAuthenticated(ctx, "net", validAuthenticatedArgs())
		if err != nil {
			return err
		}

		done := make(chan struct{})
		pulumi.All(
			pair.RelayHostname, pair.RelayUrl, pair.RelayIpv4Address,
			pair.AuthHostname, pair.AuthUrl, pair.AuthIpv4Address, pair.AuthOpsUrl,
		).ApplyT(func(o []interface{}) error {
			defer close(done)
			for i, want := range []string{
				"relay.example.test",
				"https://relay.example.test",
				"203.0.113.10",
				"auth.example.test",
				"https://auth.example.test",
				"203.0.113.10",
				"https://auth.example.test/ops/auth",
			} {
				if got := o[i].(string); got != want {
					t.Errorf("output %d = %q, want %q", i, got, want)
				}
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// Both hosts share one set of credentials and one contact address, so a missing
// one should fail once, before anything is created.
func TestNewAuthenticatedRejectsMissingRequiredArgs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*AuthenticatedArgs)
		want   string
	}{
		{"no relay hostname", func(a *AuthenticatedArgs) { a.RelayHostname = nil }, "RelayHostname is required"},
		{"no auth hostname", func(a *AuthenticatedArgs) { a.AuthHostname = nil }, "AuthHostname is required"},
		{"no contact email", func(a *AuthenticatedArgs) { a.ContactEmail = nil }, "ContactEmail is required"},
		{"no session secret", func(a *AuthenticatedArgs) { a.SessionSecret = nil }, "SessionSecret is required"},
		{"no github team", func(a *AuthenticatedArgs) { a.GithubTeam = nil }, "GithubTeam is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := validAuthenticatedArgs()
				tt.mutate(args)
				_, err := NewAuthenticated(ctx, "net", args)
				return err
			}, pulumi.WithMocks("network-services", "test", mocks(0)))

			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

// Two hosts sharing one name would race each other for the same certificate and
// then exhaust Let's Encrypt's duplicate limit for it.
func TestNewAuthenticatedRejectsIdenticalHostnames(t *testing.T) {
	for _, tt := range []struct{ name, relay, auth string }{
		{"identical", "same.example.test", "same.example.test"},
		// DNS is case-insensitive, so these are one identity, not two.
		{"differing only by case", "Same.Example.Test", "same.example.test"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := validAuthenticatedArgs()
				args.RelayHostname = pulumi.String(tt.relay)
				args.AuthHostname = pulumi.String(tt.auth)
				_, err := NewAuthenticated(ctx, "net", args)
				return err
			}, pulumi.WithMocks("network-services", "test", mocks(0)))

			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), "separate DNS names") {
				t.Errorf("error %q does not explain the hostname clash", err.Error())
			}
		})
	}
}

func TestNewAuthenticatedRejectsNilArgs(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := NewAuthenticated(ctx, "net", nil)
		return err
	}, pulumi.WithMocks("network-services", "test", mocks(0)))

	if err == nil || !strings.Contains(err.Error(), "args is required") {
		t.Errorf("expected an args-is-required error, got %v", err)
	}
}

// The hook argument is usable directly, for a relay pointed at an auth server this
// library did not create.
func TestNewAcceptsAnExplicitAuthHookServer(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "explicit-hook", &Args{
			Hostname:       pulumi.String("relay.example.test"),
			ContactEmail:   pulumi.String("ops@example.test"),
			AuthHookServer: pulumi.String("https://auth.elsewhere.test"),
		})
		if err != nil {
			return err
		}

		done := make(chan struct{})
		relay.Droplet.UserData.ApplyT(func(userData *string) error {
			defer close(done)
			if userData == nil {
				t.Errorf("droplet has no user data")
				return nil
			}
			if !strings.Contains(*userData, "--authentication-hook-server https://auth.elsewhere.test") {
				t.Errorf("explicit hook not rendered:\n%s", *userData)
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// Plain http is refused: the tokens the hook returns cross the public internet.
func TestRenderCloudInitRejectsBadAuthHookServer(t *testing.T) {
	for _, tt := range []struct {
		name, value, wantMsg string
	}{
		{"plain http", "http://auth.example.test", "must be an https:// URL"},
		{"no scheme", "auth.example.test", "must be an https:// URL"},
		{"whitespace splits the argument", "https://auth.example.test --evil", "contains whitespace"},
		{"line break", "https://auth.example.test\nExec=/bin/sh", "contains whitespace"},
		{"invalid host", "https://not_a_host", "invalid host"},
		// The text before "@" is userinfo; the real host is what follows it, so
		// picking the host out by string operations validates the wrong thing.
		{"userinfo hides the real host", "https://valid.example:443@not_a_host", "must not contain credentials"},
		{"invalid port", "https://auth.example.test:not-a-port", "is not a valid URL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderCloudInit(templateData{
				Hostname:       "relay.example.test",
				ContactEmail:   "ops@example.test",
				ContainerImage: DefaultContainerImage,
				RustLog:        "warn",
				AuthHookServer: tt.value,
			})
			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
