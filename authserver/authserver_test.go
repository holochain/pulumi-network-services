package authserver

import (
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks int

func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Copy()
	switch args.TypeToken {
	case "digitalocean:index/droplet:Droplet":
		outputs["ipv4Address"] = resource.NewStringProperty("203.0.113.20")
		outputs["ipv6Address"] = resource.NewStringProperty("2001:db8::20")
		// Numeric, because the firewall parses the droplet ID as an integer.
		return "654321", outputs, nil
	case "digitalocean:index/databaseCluster:DatabaseCluster":
		outputs["privateUri"] = resource.NewStringProperty(
			"rediss://default:pw@private-db.example.test:25061")
		outputs["uri"] = resource.NewStringProperty(
			"rediss://default:pw@public-db.example.test:25061")
	}
	return args.Name + "_id", outputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func run(t *testing.T, body func(ctx *pulumi.Context) error) {
	t.Helper()
	if err := pulumi.RunErr(body, pulumi.WithMocks("network-services", "test", mocks(0))); err != nil {
		t.Fatalf("pulumi program failed: %v", err)
	}
}

func awaitApply(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("ApplyT callback never ran: an output it depends on was never resolved")
	}
}

func validArgs() *Args {
	return &Args{
		Hostname:           pulumi.String("auth.example.test"),
		ContactEmail:       pulumi.String("ops@example.test"),
		GithubClientId:     pulumi.String("Iv1.0123456789abcdef"),
		GithubClientSecret: pulumi.String("0123456789abcdef0123456789abcdef01234567"),
		GithubOrg:          pulumi.String("holochain"),
		GithubTeam:         pulumi.String("auth"),
		SessionSecret:      pulumi.String(strings.Repeat("s", minSessionSecretBytes)),
		ApiTokens:          pulumi.String("token1,token2"),
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		auth, err := New(ctx, "auth", validArgs())
		if err != nil {
			return err
		}

		done := make(chan struct{})
		pulumi.All(
			auth.Droplet.UserData,
			auth.Droplet.Region,
			auth.Droplet.Size,
			auth.Database.Region,
			auth.Database.Engine,
			auth.Url,
			auth.OpsUrl,
		).ApplyT(func(o []interface{}) error {
			defer close(done)

			userData, ok := o[0].(*string)
			if !ok || userData == nil {
				t.Errorf("droplet has no user data")
			} else if !strings.Contains(*userData, "Image="+DefaultContainerImage+"\n") {
				t.Errorf("user data does not use DefaultContainerImage:\n%s", *userData)
			}

			if got := o[1].(string); got != defaultRegion {
				t.Errorf("droplet region = %q, want %q", got, defaultRegion)
			}
			if got := o[2].(string); got != defaultSize {
				t.Errorf("size = %q, want %q", got, defaultSize)
			}
			// Q6: the pair shares one region. A database in another region would
			// put every Valkey round-trip across the public internet.
			if got := o[3].(string); got != defaultRegion {
				t.Errorf("database region = %q, want %q — must match the droplet", got, defaultRegion)
			}
			if got := o[4].(string); got != databaseEngine {
				t.Errorf("database engine = %q, want %q", got, databaseEngine)
			}
			if got := o[5].(string); got != "https://auth.example.test" {
				t.Errorf("url = %q", got)
			}
			if got := o[6].(string); got != "https://auth.example.test/ops/auth" {
				t.Errorf("opsUrl = %q", got)
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// The database URL must be the private endpoint, so Valkey traffic stays on
// DigitalOcean's network instead of crossing the public internet.
func TestNewUsesThePrivateDatabaseEndpoint(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		auth, err := New(ctx, "private-db", validArgs())
		if err != nil {
			return err
		}

		done := make(chan struct{})
		auth.Droplet.UserData.ApplyT(func(userData *string) error {
			defer close(done)
			if userData == nil {
				t.Errorf("droplet has no user data")
				return nil
			}
			if !strings.Contains(*userData, "REDIS_URL=rediss://default:pw@private-db.example.test:25061") {
				t.Errorf("REDIS_URL is not the private endpoint:\n%s", *userData)
			}
			if strings.Contains(*userData, "public-db.example.test") {
				t.Errorf("user data uses the public database endpoint:\n%s", *userData)
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

func TestNewOpensOnlyTheeNeededPorts(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		auth, err := New(ctx, "ports", validArgs())
		if err != nil {
			return err
		}

		done := make(chan struct{})
		auth.Firewall.InboundRules.ApplyT(func(in []digitalocean.FirewallInboundRule) error {
			defer close(done)

			seen := map[string]bool{}
			for _, r := range in {
				port := ""
				if r.PortRange != nil {
					port = *r.PortRange
				}
				seen[r.Protocol+"/"+port] = true
			}
			for _, want := range []string{"tcp/443", "tcp/80", "icmp/"} {
				if !seen[want] {
					t.Errorf("firewall is missing %s; got %v", want, seen)
				}
			}
			// No UDP listener on this host, unlike the relay.
			for k := range seen {
				if strings.HasPrefix(k, "udp/") {
					t.Errorf("unexpected udp rule %s on the auth server", k)
				}
			}
			if seen["tcp/22"] {
				t.Errorf("port 22 is open even though no SshKeys were attached")
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// Attaching keys opens SSH from anywhere by default. Operators have dynamic
// addresses, and a source list that must be rotated is one that goes stale.
func TestNewOpensSshToAnySourceWhenKeysAreAttachedWithoutSources(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		args := validArgs()
		args.SshKeys = pulumi.StringArray{pulumi.String("aa:bb:cc")}
		auth, err := New(ctx, "keys-no-sources", args)
		if err != nil {
			return err
		}

		done := make(chan struct{})
		auth.Firewall.InboundRules.ApplyT(func(in []digitalocean.FirewallInboundRule) error {
			defer close(done)
			for _, r := range in {
				if r.PortRange != nil && *r.PortRange == "22" {
					if len(r.SourceAddresses) != 2 {
						t.Errorf("ssh sources = %v, want v4 and v6 anywhere", r.SourceAddresses)
					}
					return nil
				}
			}
			t.Errorf("no ssh rule despite attached keys")
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

// A source list still narrows it when the addresses are worth maintaining.
func TestNewNarrowsSshToGivenSources(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		args := validArgs()
		args.SshKeys = pulumi.StringArray{pulumi.String("aa:bb:cc")}
		args.SshSourceAddresses = pulumi.StringArray{pulumi.String("203.0.113.0/24")}
		auth, err := New(ctx, "narrowed", args)
		if err != nil {
			return err
		}

		done := make(chan struct{})
		auth.Firewall.InboundRules.ApplyT(func(in []digitalocean.FirewallInboundRule) error {
			defer close(done)
			for _, r := range in {
				if r.PortRange != nil && *r.PortRange == "22" {
					if len(r.SourceAddresses) != 1 || r.SourceAddresses[0] != "203.0.113.0/24" {
						t.Errorf("ssh sources = %v, want [203.0.113.0/24]", r.SourceAddresses)
					}
					return nil
				}
			}
			t.Errorf("no ssh rule despite attached keys")
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}

func TestNewRejectsMissingRequiredArgs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Args)
		want   string
	}{
		{"no hostname", func(a *Args) { a.Hostname = nil }, "Hostname is required"},
		{"no github client id", func(a *Args) { a.GithubClientId = nil }, "GithubClientId is required"},
		{"no github team", func(a *Args) { a.GithubTeam = nil }, "GithubTeam is required"},
		{"no session secret", func(a *Args) { a.SessionSecret = nil }, "SessionSecret is required"},
		{"no api tokens", func(a *Args) { a.ApiTokens = nil }, "ApiTokens is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				args := validArgs()
				tt.mutate(args)
				_, err := New(ctx, "invalid", args)
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
