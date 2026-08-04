// Package bootstraprelay provisions a Kitsune2 bootstrap/relay server on
// DigitalOcean.
//
// From Kitsune2 0.5.0, peer discovery (bootstrap) and connection establishment
// (relay) are a single service, so one component covers both roles.
//
// The component creates a droplet and a firewall restricting it to the ports the
// service needs. The firewall is not optional.
//
// It does not create DNS records: pass the hostname you intend to use and wire
// A/AAAA records from Ipv4Address and Ipv6Address with whatever DNS provider you
// use. Certbot on the host retries with backoff, though not indefinitely, so
// records created in the same update are fine if they resolve within the window.
package bootstraprelay

import (
	"errors"

	"github.com/holochain/pulumi-network-services/internal/dofirewall"
	"github.com/holochain/pulumi-network-services/internal/inputs"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// resourceType is the Pulumi type token for this component. It forms part of every
// URN the component produces, so it must not change: doing so would replace every
// resource built from it.
const resourceType = "holochain:bootstraprelay:BootstrapRelay"

// Args configures a BootstrapRelay.
type Args struct {
	// Hostname is the fully qualified domain name the server will be reached on.
	// It drives certbot, the TLS certificate paths and the listen configuration.
	// Required.
	Hostname pulumi.StringInput

	// ContactEmail is the address registered with Let's Encrypt. Required.
	ContactEmail pulumi.StringInput

	// ContainerImage is the kitsune2_bootstrap_srv container image to run.
	// Defaults to DefaultContainerImage. Pin this in production: leaving it unset
	// means a MINOR release of this library can replace your droplet.
	ContainerImage pulumi.StringInput

	// Region is the DigitalOcean region slug. Defaults to fra1.
	Region pulumi.StringInput

	// Size is the DigitalOcean droplet size slug. Defaults to s-2vcpu-4gb, which
	// is sized for production; override it for test deployments.
	Size pulumi.StringInput

	// SshKeys are DigitalOcean SSH key fingerprints to authorise for root. Empty
	// by default: this component never attaches keys you did not ask for.
	SshKeys pulumi.StringArrayInput

	// SshSourceAddresses narrows which CIDRs may reach SSH. Optional: when
	// SshKeys is set and this is empty, port 22 is open to any source, which is
	// what an operator with a dynamic address needs. Set it when the addresses
	// are stable enough to be worth maintaining.
	SshSourceAddresses pulumi.StringArrayInput

	// Tags are applied to the droplet.
	Tags pulumi.StringArrayInput

	// Ipv6 enables IPv6 on the droplet. Defaults to true.
	Ipv6 pulumi.BoolInput

	// Monitoring enables the DigitalOcean monitoring agent. Defaults to true.
	Monitoring pulumi.BoolInput

	// RustLog sets RUST_LOG for the service. Defaults to "warn".
	RustLog pulumi.StringInput

	// AuthHookServer is the base URL of an auth server implementing the sbd
	// authentication hook specification. When set, peers must authenticate before
	// using this relay. Must be an https:// URL: the tokens it returns cross the
	// public internet.
	//
	// Prefer NewAuthenticated over setting this by hand — it provisions both hosts
	// and wires this from the auth server's own URL.
	AuthHookServer pulumi.StringInput

	// ExtraArgs are appended to the kitsune2-bootstrap-srv command line.
	ExtraArgs pulumi.StringArrayInput
}

// BootstrapRelay is a Kitsune2 bootstrap/relay server running on a DigitalOcean
// droplet, provisioned by cloud-init and served over TLS by a Let's Encrypt
// certificate obtained on first boot.
type BootstrapRelay struct {
	pulumi.ResourceState

	// Droplet is the underlying DigitalOcean droplet.
	Droplet *digitalocean.Droplet

	// Firewall restricts inbound traffic to the ports the service needs.
	Firewall *digitalocean.Firewall

	Ipv4Address pulumi.StringOutput
	Ipv6Address pulumi.StringOutput
	Hostname    pulumi.StringOutput

	// Url is the https:// endpoint clients connect to.
	Url pulumi.StringOutput
}

// New provisions a bootstrap/relay server.
//
// Changing any argument that affects cloud-init replaces the droplet: user data is
// immutable on DigitalOcean. Replacement means a new IP address and a fresh
// Let's Encrypt issuance, so keep DNS TTLs low and be aware that Let's Encrypt
// permits five duplicate certificates per week for a given hostname.
func New(ctx *pulumi.Context, name string, args *Args, opts ...pulumi.ResourceOption) (*BootstrapRelay, error) {
	if args == nil {
		return nil, errors.New("bootstraprelay.New: args is required")
	}
	if args.Hostname == nil {
		return nil, errors.New("bootstraprelay.New: Hostname is required")
	}
	if args.ContactEmail == nil {
		return nil, errors.New("bootstraprelay.New: ContactEmail is required")
	}

	component := &BootstrapRelay{}
	if err := ctx.RegisterComponentResource(resourceType, name, component, opts...); err != nil {
		return nil, err
	}

	hostname := args.Hostname.ToStringOutput()

	userData := pulumi.All(
		hostname,
		args.ContactEmail.ToStringOutput(),
		inputs.StringOr(args.ContainerImage, DefaultContainerImage),
		inputs.StringOr(args.RustLog, defaultRustLog),
		inputs.StringOr(args.AuthHookServer, ""),
		inputs.StringArrayOrEmpty(args.ExtraArgs),
	).ApplyT(func(templateArgs []interface{}) (string, error) {
		return renderCloudInit(templateData{
			Hostname:       templateArgs[0].(string),
			ContactEmail:   templateArgs[1].(string),
			ContainerImage: templateArgs[2].(string),
			RustLog:        templateArgs[3].(string),
			AuthHookServer: templateArgs[4].(string),
			ExtraArgs:      templateArgs[5].([]string),
		})
	}).(pulumi.StringOutput)

	droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
		Name:       pulumi.String(name),
		Image:      pulumi.String(defaultOsImage),
		Region:     inputs.StringOr(args.Region, defaultRegion),
		Size:       inputs.StringOr(args.Size, defaultSize),
		Ipv6:       inputs.BoolPtrOr(args.Ipv6, true),
		Monitoring: inputs.BoolPtrOr(args.Monitoring, true),
		Tags:       inputs.StringArrayOrEmpty(args.Tags),
		SshKeys:    inputs.StringArrayOrEmpty(args.SshKeys),
		UserData:   userData,
	}, pulumi.Parent(component))
	if err != nil {
		return nil, err
	}

	firewall, err := dofirewall.New(ctx, name, component, droplet, dofirewall.Args{
		Ports: []dofirewall.Port{
			dofirewall.TCP(portHttps),
			dofirewall.UDP(portQad),
			dofirewall.TCP(portHttp),
			dofirewall.ICMP,
		},
		SshPort:            portSsh,
		AllowSsh:           args.SshKeys != nil,
		SshSourceAddresses: args.SshSourceAddresses,
	})
	if err != nil {
		return nil, err
	}

	component.Droplet = droplet
	component.Firewall = firewall
	component.Ipv4Address = droplet.Ipv4Address
	component.Ipv6Address = droplet.Ipv6Address
	component.Hostname = hostname
	component.Url = hostname.ApplyT(func(h string) string {
		return "https://" + h
	}).(pulumi.StringOutput)

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"ipv4Address": component.Ipv4Address,
		"ipv6Address": component.Ipv6Address,
		"hostname":    component.Hostname,
		"url":         component.Url,
	}); err != nil {
		return nil, err
	}

	return component, nil
}
