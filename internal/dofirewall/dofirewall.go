// Package dofirewall builds the DigitalOcean firewall shared by the components in
// this repository.
//
// Each component knows which ports its own service binds; everything else — egress,
// ICMP, SSH, and the droplet wiring — is identical between them and lives here.
package dofirewall

import (
	"fmt"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// anywhere is every source, v4 and v6. These are public services: the firewall
// limits which ports are reachable, not from where.
var anywhere = pulumi.StringArray{
	pulumi.String("0.0.0.0/0"),
	pulumi.String("::/0"),
}

// Port is one inbound port a service needs open.
type Port struct {
	// Protocol is "tcp", "udp" or "icmp".
	Protocol string

	// Number is the port. Ignored when Protocol is "icmp", which has no ports.
	Number int
}

// TCP returns a tcp Port.
func TCP(number int) Port { return Port{Protocol: "tcp", Number: number} }

// UDP returns a udp Port.
func UDP(number int) Port { return Port{Protocol: "udp", Number: number} }

// ICMP is required for Path MTU Discovery. Blocking it black-holes large packets
// intermittently, which is disproportionately hard to diagnose.
var ICMP = Port{Protocol: "icmp"}

// Args configures New.
type Args struct {
	// Ports the component's own service binds.
	Ports []Port

	// SshPort is opened when AllowSsh is set.
	SshPort int

	// AllowSsh should be true when SSH keys are attached to the droplet.
	//
	// Without keys there is nothing to open: DigitalOcean only sets a root
	// password when no keys are attached, so leaving 22 open in that case would
	// expose password authentication.
	AllowSsh bool

	// SshSourceAddresses narrows which CIDRs may reach SSH. When empty and
	// AllowSsh is set, SSH is open to every source: operators have dynamic
	// addresses, and a source list that must be rotated is one that goes stale or
	// gets widened in a hurry. The key pair is the control, not the CIDR.
	SshSourceAddresses pulumi.StringArrayInput
}

// New creates a firewall restricting the droplet to the given ports.
func New(
	ctx *pulumi.Context,
	name string,
	parent pulumi.Resource,
	droplet *digitalocean.Droplet,
	args Args,
) (*digitalocean.Firewall, error) {
	inbound := digitalocean.FirewallInboundRuleArray{}
	for _, p := range args.Ports {
		rule := &digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String(p.Protocol),
			SourceAddresses: anywhere,
		}
		if p.Protocol != "icmp" {
			rule.PortRange = pulumi.String(fmt.Sprint(p.Number))
		}
		inbound = append(inbound, rule)
	}

	if args.AllowSsh {
		sources := args.SshSourceAddresses
		if sources == nil {
			sources = anywhere
		}
		inbound = append(inbound, &digitalocean.FirewallInboundRuleArgs{
			Protocol:        pulumi.String("tcp"),
			PortRange:       pulumi.String(fmt.Sprint(args.SshPort)),
			SourceAddresses: sources,
		})
	}

	// A DigitalOcean firewall with no outbound rules denies all egress, which
	// would strand cloud-init before it finished: apt, the container registry and
	// the ACME challenge all need to get out. Narrowing it would mean tracking
	// registry and ACME endpoints that move.
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
