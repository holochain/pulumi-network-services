package bootstraprelay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/holochain/pulumi-network-services/authserver"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// authenticatedResourceType is the Pulumi type token for the paired component. It
// forms part of every URN it produces, so it must not change.
const authenticatedResourceType = "holochain:bootstraprelay:Authenticated"

// AuthenticatedArgs configures an Authenticated pair.
//
// The two hosts share one Region rather than taking one each. Splitting them would
// put every authentication round-trip across the public internet for no benefit,
// and it is not a mistake worth leaving available.
//
// Arguments are flat rather than nested per component. Nesting the existing Args
// structs would have meant two more Region fields that had to be ignored, which is
// exactly the kind of silently-discarded input this avoids. Fields that both hosts
// share are single; fields that differ carry a Relay or Auth prefix; fields that
// only make sense for one of them are bare.
type AuthenticatedArgs struct {
	// RelayHostname and AuthHostname are the fully qualified domain names of the
	// two hosts. Both required, and they must differ.
	RelayHostname pulumi.StringInput
	AuthHostname  pulumi.StringInput

	// ContactEmail is registered with Let's Encrypt for both certificates.
	// Required.
	ContactEmail pulumi.StringInput

	// GitHub OAuth application backing the auth server's ops console. Required.
	GithubClientId     pulumi.StringInput
	GithubClientSecret pulumi.StringInput

	// GithubOrg and GithubTeam decide who may approve or block keys. Required.
	GithubOrg  pulumi.StringInput
	GithubTeam pulumi.StringInput

	// SessionSecret signs ops console session cookies. Required, at least 64 bytes.
	SessionSecret pulumi.StringInput

	// ApiTokens is the comma-separated list of bearer tokens accepted by the auth
	// server's administrative API. Required.
	ApiTokens pulumi.StringInput

	// Region hosts both droplets and the database. Defaults to fra1.
	Region pulumi.StringInput

	// Container images. Pin both in production.
	RelayContainerImage pulumi.StringInput
	AuthContainerImage  pulumi.StringInput

	// Droplet sizes.
	RelaySize pulumi.StringInput
	AuthSize  pulumi.StringInput

	// DatabaseSize is the managed Valkey cluster size slug.
	DatabaseSize pulumi.StringInput

	// MaxPendingRequests caps how many unapproved key requests are retained.
	MaxPendingRequests pulumi.IntInput

	// RustLog sets RUST_LOG on the relay.
	RustLog pulumi.StringInput

	// RelayExtraArgs are appended to the kitsune2-bootstrap-srv command line.
	RelayExtraArgs pulumi.StringArrayInput

	// SshKeys and SshSourceAddresses apply to both droplets.
	SshKeys            pulumi.StringArrayInput
	SshSourceAddresses pulumi.StringArrayInput

	// Tags are applied to both droplets.
	Tags pulumi.StringArrayInput

	// Ipv6 and Monitoring apply to both droplets.
	Ipv6       pulumi.BoolInput
	Monitoring pulumi.BoolInput
}

// Authenticated is a bootstrap/relay server paired with the auth server that gates
// access to it.
//
// The relay's authentication hook is wired to the auth server's URL by the
// component, so the two cannot drift apart.
type Authenticated struct {
	pulumi.ResourceState

	// Relay and Auth are the underlying components, exposed for anything these
	// flattened outputs do not cover.
	Relay *BootstrapRelay
	Auth  *authserver.AuthServer

	RelayHostname    pulumi.StringOutput
	RelayUrl         pulumi.StringOutput
	RelayIpv4Address pulumi.StringOutput
	RelayIpv6Address pulumi.StringOutput

	AuthHostname    pulumi.StringOutput
	AuthUrl         pulumi.StringOutput
	AuthIpv4Address pulumi.StringOutput
	AuthIpv6Address pulumi.StringOutput

	// AuthOpsUrl is the console operators use to approve keys.
	AuthOpsUrl pulumi.StringOutput
}

// NewAuthenticated provisions a bootstrap/relay server and the auth server that
// gates it.
//
// The auth server is created first: the relay needs its URL, so the dependency
// runs in that direction and Pulumi orders them accordingly.
//
// Both hosts still need DNS records of their own. Create A/AAAA records for each
// from the exported addresses, remembering that certbot on each host has a bounded
// retry window.
func NewAuthenticated(
	ctx *pulumi.Context,
	name string,
	args *AuthenticatedArgs,
	opts ...pulumi.ResourceOption,
) (*Authenticated, error) {
	if args == nil {
		return nil, errors.New("bootstraprelay.NewAuthenticated: args is required")
	}
	for _, required := range []struct {
		name  string
		value pulumi.StringInput
	}{
		{"RelayHostname", args.RelayHostname},
		{"AuthHostname", args.AuthHostname},
		{"ContactEmail", args.ContactEmail},
		{"GithubClientId", args.GithubClientId},
		{"GithubClientSecret", args.GithubClientSecret},
		{"GithubOrg", args.GithubOrg},
		{"GithubTeam", args.GithubTeam},
		{"SessionSecret", args.SessionSecret},
		{"ApiTokens", args.ApiTokens},
	} {
		if required.value == nil {
			return nil, errors.New("bootstraprelay.NewAuthenticated: " + required.name + " is required")
		}
	}

	component := &Authenticated{}
	if err := ctx.RegisterComponentResource(authenticatedResourceType, name, component, opts...); err != nil {
		return nil, err
	}

	// The two hostnames must differ, and neither is a plain string, so the check
	// runs where they resolve. Threading the result into the auth server's own
	// hostname makes it gate the first child rather than sitting to one side:
	// nothing is created if it fails, and `pulumi preview` surfaces it before any
	// resource exists.
	//
	// Sharing a hostname would not merely confuse DNS. Both hosts would run
	// certbot for the same name, and Let's Encrypt allows five duplicate
	// certificates per week — so the pair would race, and then lock the name out.
	authHostname := pulumi.All(
		args.RelayHostname.ToStringOutput(),
		args.AuthHostname.ToStringOutput(),
	).ApplyT(func(hostnames []interface{}) (string, error) {
		relayHostname := hostnames[0].(string)
		authHostname := hostnames[1].(string)
		// DNS names are case-insensitive, so Relay.Example and relay.example are
		// the same identity.
		if strings.EqualFold(relayHostname, authHostname) {
			return "", fmt.Errorf(
				"bootstraprelay.NewAuthenticated: RelayHostname and AuthHostname are both %q, "+
					"but the two hosts need separate DNS names and separate certificates",
				relayHostname)
		}
		return authHostname, nil
	}).(pulumi.StringOutput)

	auth, err := authserver.New(ctx, name+"-auth", &authserver.Args{
		Hostname:           authHostname,
		ContactEmail:       args.ContactEmail,
		GithubClientId:     args.GithubClientId,
		GithubClientSecret: args.GithubClientSecret,
		GithubOrg:          args.GithubOrg,
		GithubTeam:         args.GithubTeam,
		SessionSecret:      args.SessionSecret,
		ApiTokens:          args.ApiTokens,
		ContainerImage:     args.AuthContainerImage,
		Region:             args.Region,
		Size:               args.AuthSize,
		DatabaseSize:       args.DatabaseSize,
		MaxPendingRequests: args.MaxPendingRequests,
		SshKeys:            args.SshKeys,
		SshSourceAddresses: args.SshSourceAddresses,
		Tags:               args.Tags,
		Ipv6:               args.Ipv6,
		Monitoring:         args.Monitoring,
	}, pulumi.Parent(component))
	if err != nil {
		return nil, err
	}

	relay, err := New(ctx, name+"-relay", &Args{
		Hostname:       args.RelayHostname,
		ContactEmail:   args.ContactEmail,
		ContainerImage: args.RelayContainerImage,
		Region:         args.Region,
		Size:           args.RelaySize,
		RustLog:        args.RustLog,
		// Wired from the component rather than left to the caller, so the relay
		// cannot end up pointing at the wrong auth server, or at none.
		AuthHookServer:     auth.Url,
		ExtraArgs:          args.RelayExtraArgs,
		SshKeys:            args.SshKeys,
		SshSourceAddresses: args.SshSourceAddresses,
		Tags:               args.Tags,
		Ipv6:               args.Ipv6,
		Monitoring:         args.Monitoring,
	}, pulumi.Parent(component))
	if err != nil {
		return nil, err
	}

	component.Relay = relay
	component.Auth = auth

	component.RelayHostname = relay.Hostname
	component.RelayUrl = relay.Url
	component.RelayIpv4Address = relay.Ipv4Address
	component.RelayIpv6Address = relay.Ipv6Address

	component.AuthHostname = auth.Hostname
	component.AuthUrl = auth.Url
	component.AuthIpv4Address = auth.Ipv4Address
	component.AuthIpv6Address = auth.Ipv6Address
	component.AuthOpsUrl = auth.OpsUrl

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"relayHostname":    component.RelayHostname,
		"relayUrl":         component.RelayUrl,
		"relayIpv4Address": component.RelayIpv4Address,
		"relayIpv6Address": component.RelayIpv6Address,
		"authHostname":     component.AuthHostname,
		"authUrl":          component.AuthUrl,
		"authIpv4Address":  component.AuthIpv4Address,
		"authIpv6Address":  component.AuthIpv6Address,
		"authOpsUrl":       component.AuthOpsUrl,
	}); err != nil {
		return nil, err
	}

	return component, nil
}
