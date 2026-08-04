package bootstraprelay

import (
	"testing"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// rule is a flattened firewall rule, so assertions read as protocol/port rather
// than as index arithmetic over the SDK types.
type rule struct {
	protocol string
	port     string
}

func inboundRules(t *testing.T, relay *BootstrapRelay) []rule {
	t.Helper()

	var rules []rule
	done := make(chan struct{})
	relay.Firewall.InboundRules.ApplyT(func(in []digitalocean.FirewallInboundRule) error {
		defer close(done)
		for _, r := range in {
			port := ""
			if r.PortRange != nil {
				port = *r.PortRange
			}
			rules = append(rules, rule{protocol: r.Protocol, port: port})
		}
		return nil
	})
	awaitApply(t, done)
	return rules
}

func hasRule(rules []rule, protocol, port string) bool {
	for _, r := range rules {
		if r.protocol == protocol && r.port == port {
			return true
		}
	}
	return false
}

// The two rules most easily missed by hand both fail quietly: UDP for QUIC
// address discovery degrades every peer onto relaying, and port 80 stops certbot
// renewing ninety days later.
func TestNewOpensEveryPortTheServiceNeeds(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "ports", &Args{
			Hostname:     pulumi.String("relay.example.test"),
			ContactEmail: pulumi.String("ops@example.test"),
		})
		if err != nil {
			return err
		}

		rules := inboundRules(t, relay)
		for _, want := range []rule{
			{protocol: "tcp", port: "443"},
			{protocol: "udp", port: "7842"},
			{protocol: "tcp", port: "80"},
			{protocol: "icmp", port: ""},
		} {
			if !hasRule(rules, want.protocol, want.port) {
				t.Errorf("firewall is missing %s/%s; got %v", want.protocol, want.port, rules)
			}
		}
		return nil
	})
}

// No keys means no SSH rule: DigitalOcean sets a root password when a droplet has
// no keys, so an open port 22 would expose password authentication.
func TestNewClosesSshWhenNoKeysAttached(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "no-ssh", &Args{
			Hostname:     pulumi.String("relay.example.test"),
			ContactEmail: pulumi.String("ops@example.test"),
		})
		if err != nil {
			return err
		}

		if rules := inboundRules(t, relay); hasRule(rules, "tcp", "22") {
			t.Errorf("port 22 is open with no SshKeys attached; got %v", rules)
		}
		return nil
	})
}

// Attaching keys opens SSH from anywhere by default, for operators on dynamic
// addresses. A source list narrows it when the addresses are worth maintaining.
func TestNewOpensSshOnlyToGivenSources(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "with-ssh", &Args{
			Hostname:           pulumi.String("relay.example.test"),
			ContactEmail:       pulumi.String("ops@example.test"),
			SshKeys:            pulumi.StringArray{pulumi.String("aa:bb:cc")},
			SshSourceAddresses: pulumi.StringArray{pulumi.String("203.0.113.0/24")},
		})
		if err != nil {
			return err
		}

		if rules := inboundRules(t, relay); !hasRule(rules, "tcp", "22") {
			t.Errorf("port 22 is not open despite SshSourceAddresses; got %v", rules)
		}
		return nil
	})
}

// Keys with no source list open port 22 to every source, which is what an
// operator with a dynamic address needs.
func TestNewOpensSshToAnySourceWhenKeysHaveNoSources(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "keys-no-sources", &Args{
			Hostname:     pulumi.String("relay.example.test"),
			ContactEmail: pulumi.String("ops@example.test"),
			SshKeys:      pulumi.StringArray{pulumi.String("aa:bb:cc")},
		})
		if err != nil {
			return err
		}

		if rules := inboundRules(t, relay); !hasRule(rules, "tcp", "22") {
			t.Errorf("port 22 is not open despite attached keys; got %v", rules)
		}
		return nil
	})
}

// Egress must stay open or cloud-init cannot finish: a DigitalOcean firewall with
// no outbound rules denies all egress.
func TestNewAllowsEgress(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "egress", &Args{
			Hostname:     pulumi.String("relay.example.test"),
			ContactEmail: pulumi.String("ops@example.test"),
		})
		if err != nil {
			return err
		}

		done := make(chan struct{})
		relay.Firewall.OutboundRules.ApplyT(func(out []digitalocean.FirewallOutboundRule) error {
			defer close(done)
			if len(out) == 0 {
				t.Errorf("no outbound rules: DigitalOcean would deny all egress and cloud-init could not complete")
			}
			for _, r := range out {
				if len(r.DestinationAddresses) == 0 {
					t.Errorf("outbound %s rule has no destinations", r.Protocol)
				}
			}
			return nil
		})
		awaitApply(t, done)
		return nil
	})
}
