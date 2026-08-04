package bootstraprelay

import (
	"fmt"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// anywhere is every source, v4 and v6. The bootstrap/relay is a public service:
// the point of the firewall is to limit which ports are reachable, not from where.
var anywhere = pulumi.StringArray{
	pulumi.String("0.0.0.0/0"),
	pulumi.String("::/0"),
}

// newFirewall restricts the droplet to the ports the service actually needs.
//
// The firewall is not optional. A consumer would have to rediscover this port
// matrix to write it themselves, and the two entries most easily missed — UDP for
// QUIC address discovery, and port 80 for certbot renewals — both fail quietly
// rather than loudly.
func newFirewall(
	ctx *pulumi.Context,
	name string,
	parent pulumi.Resource,
	droplet *digitalocean.Droplet,
	allowSsh bool,
	sshSourceAddresses pulumi.StringArrayInput,
) (*digitalocean.Firewall, error) {
	inbound := digitalocean.FirewallInboundRuleArray{
		&digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("tcp"),
			PortRange:       pulumi.String(fmt.Sprint(portHttps)),
			SourceAddresses: anywhere,
		},
		&digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("udp"),
			PortRange:       pulumi.String(fmt.Sprint(portQad)),
			SourceAddresses: anywhere,
		},
		&digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("tcp"),
			PortRange:       pulumi.String(fmt.Sprint(portHttp)),
			SourceAddresses: anywhere,
		},
		// Dropping ICMP breaks Path MTU Discovery, and a QUIC service that cannot
		// discover the path MTU black-holes large packets intermittently. That is
		// a genuinely miserable failure to diagnose, so it stays open.
		&digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("icmp"),
			SourceAddresses: anywhere,
		},
	}

	// SSH is opened whenever keys are attached, because keys nothing can reach are
	// useless. It defaults to every source: operators have dynamic addresses, and
	// a source list that has to be rotated is one that goes stale or gets widened
	// in a hurry. The key pair is the control here, not the CIDR.
	//
	// Without keys there is nothing to open: DigitalOcean only sets a root
	// password when no keys are attached, so leaving 22 open in that case would
	// expose password authentication.
	if allowSsh {
		sources := sshSourceAddresses
		if sources == nil {
			sources = anywhere
		}
		inbound = append(inbound, &digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("tcp"),
			PortRange:       pulumi.String(fmt.Sprint(portSsh)),
			SourceAddresses: sources,
		})
	}

	// A DigitalOcean firewall with no outbound rules denies all egress, which
	// would break cloud-init before it finished: apt, the container registry and
	// the ACME challenge all need to get out. Egress is left open deliberately —
	// narrowing it would mean tracking registry and ACME endpoints that move.
	outbound := digitalocean.FirewallOutboundRuleArray{
		&digitalocean.FirewallOutboundRuleArgs{
			Protocol:             pulumi.String("tcp"),
			PortRange:            pulumi.String("1-65535"),
			DestinationAddresses: anywhere,
		},
		&digitalocean.FirewallOutboundRuleArgs{
			Protocol:             pulumi.String("udp"),
			PortRange:            pulumi.String("1-65535"),
			DestinationAddresses: anywhere,
		},
		&digitalocean.FirewallOutboundRuleArgs{
			Protocol:             pulumi.String("icmp"),
			DestinationAddresses: anywhere,
		},
	}

	return digitalocean.NewFirewall(ctx, name, &digitalocean.FirewallArgs{
		Name:          pulumi.String(name),
		DropletIds:    pulumi.IntArray{droplet.ID().ApplyT(dropletIDToInt).(pulumi.IntOutput)},
		InboundRules:  inbound,
		OutboundRules: outbound,
	}, pulumi.Parent(parent))
}

// dropletIDToInt converts the droplet's resource ID, which Pulumi models as a
// string, into the integer the firewall expects.
func dropletIDToInt(id pulumi.ID) (int, error) {
	var n int
	if _, err := fmt.Sscanf(string(id), "%d", &n); err != nil {
		return 0, fmt.Errorf("droplet id %q is not numeric: %w", id, err)
	}
	return n, nil
}
