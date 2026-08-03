// Package bootstraprelay provisions a Kitsune2 bootstrap/relay server on
// DigitalOcean.
//
// From Kitsune2 0.5.0, peer discovery (bootstrap) and connection establishment
// (relay) are a single service, so one component covers both roles.
//
// The component creates a droplet and nothing else. It does not create DNS
// records: pass the hostname you intend to use and wire A/AAAA records from
// Ipv4Address and Ipv6Address with whatever DNS provider you use. Certbot on the
// host retries until DNS resolves, so the ordering takes care of itself.
package bootstraprelay

import (
	"errors"

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

	// Size is the DigitalOcean droplet size slug. Defaults to s-2vcpu-2gb.
	Size pulumi.StringInput

	// SshKeys are DigitalOcean SSH key fingerprints to authorise for root. Empty
	// by default: this component never attaches keys you did not ask for.
	SshKeys pulumi.StringArrayInput

	// Tags are applied to the droplet.
	Tags pulumi.StringArrayInput

	// Ipv6 enables IPv6 on the droplet. Defaults to true.
	Ipv6 pulumi.BoolInput

	// Monitoring enables the DigitalOcean monitoring agent. Defaults to true.
	Monitoring pulumi.BoolInput

	// RustLog sets RUST_LOG for the service. Defaults to "warn".
	RustLog pulumi.StringInput

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
		stringOrDefault(args.ContainerImage, DefaultContainerImage),
		stringOrDefault(args.RustLog, defaultRustLog),
		stringArrayOrEmpty(args.ExtraArgs),
	).ApplyT(func(templateArgs []interface{}) (string, error) {
		return renderCloudInit(templateData{
			Hostname:       templateArgs[0].(string),
			ContactEmail:   templateArgs[1].(string),
			ContainerImage: templateArgs[2].(string),
			RustLog:        templateArgs[3].(string),
			ExtraArgs:      templateArgs[4].([]string),
		})
	}).(pulumi.StringOutput)

	droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
		Name:       pulumi.String(name),
		Image:      pulumi.String(defaultOsImage),
		Region:     stringOrDefault(args.Region, defaultRegion),
		Size:       stringOrDefault(args.Size, defaultSize),
		Ipv6:       boolPtrOrDefault(args.Ipv6, true),
		Monitoring: boolPtrOrDefault(args.Monitoring, true),
		Tags:       stringArrayOrEmpty(args.Tags),
		SshKeys:    stringArrayOrEmpty(args.SshKeys),
		UserData:   userData,
	}, pulumi.Parent(component))
	if err != nil {
		return nil, err
	}

	component.Droplet = droplet
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

func stringOrDefault(in pulumi.StringInput, fallback string) pulumi.StringOutput {
	if in == nil {
		return pulumi.String(fallback).ToStringOutput()
	}
	return in.ToStringOutput()
}

func boolPtrOrDefault(in pulumi.BoolInput, fallback bool) pulumi.BoolPtrOutput {
	if in == nil {
		return pulumi.Bool(fallback).ToBoolPtrOutput()
	}
	return in.ToBoolOutput().ToBoolPtrOutput()
}

func stringArrayOrEmpty(in pulumi.StringArrayInput) pulumi.StringArrayOutput {
	if in == nil {
		return pulumi.StringArray{}.ToStringArrayOutput()
	}
	return in.ToStringArrayOutput()
}
