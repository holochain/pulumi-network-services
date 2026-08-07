// Package authserver provisions a Holochain auth server on DigitalOcean.
//
// The auth server implements the sbd authentication hook specification. A
// bootstrap/relay calls its /authenticate endpoint to turn a signed request into a
// token; operators approve or block keys through a GitHub-authenticated ops
// console; and agents call /now and /request-auth themselves to ask for access.
//
// Because agents and operators both reach it directly, this is a public service.
// It is not an internal component that can hide behind a private network.
//
// The component creates a droplet, a managed Valkey cluster, and firewalls for
// both. It does not create DNS records: pass the hostname you intend to use and
// wire A/AAAA records from Ipv4Address and Ipv6Address.
package authserver

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
const resourceType = "holochain:authserver:AuthServer"

// Args configures an AuthServer.
type Args struct {
	// Hostname is the fully qualified domain name the server will be reached on.
	// It drives certbot, the TLS certificate paths and the OAuth redirect URI,
	// which must match the callback URL registered with the GitHub OAuth app.
	// Required.
	Hostname pulumi.StringInput

	// ContactEmail is the address registered with Let's Encrypt. Required.
	ContactEmail pulumi.StringInput

	// GithubClientId and GithubClientSecret identify the GitHub OAuth app backing
	// the ops console. Required. Pass the secret from configuration, not a
	// literal.
	GithubClientId     pulumi.StringInput
	GithubClientSecret pulumi.StringInput

	// GithubOrg and GithubTeam decide who may use the ops console: a user must be
	// a member of this team to approve or block keys. Required.
	GithubOrg  pulumi.StringInput
	GithubTeam pulumi.StringInput

	// SessionSecret signs the ops console session cookies. Required, and at least
	// 64 bytes.
	SessionSecret pulumi.StringInput

	// ApiTokens is the comma-separated list of bearer tokens accepted by the
	// administrative API. Required.
	ApiTokens pulumi.StringInput

	// ContainerImage is the hc_auth_server image to run. Defaults to
	// DefaultContainerImage. Pin this in production: leaving it unset means a
	// MINOR release of this library can replace your droplet.
	ContainerImage pulumi.StringInput

	// Region is the DigitalOcean region slug for both the droplet and the
	// database. Defaults to fra1.
	Region pulumi.StringInput

	// Size is the DigitalOcean droplet size slug. Defaults to s-1vcpu-2gb.
	Size pulumi.StringInput

	// DatabaseSize is the managed Valkey cluster size slug. Defaults to
	// db-s-1vcpu-1gb.
	DatabaseSize pulumi.StringInput

	// MaxPendingRequests caps how many unapproved key requests are held at once.
	// Defaults to 100.
	MaxPendingRequests pulumi.IntInput

	// SshKeys are DigitalOcean SSH key fingerprints to authorise for root. Empty
	// by default: this component never attaches keys you did not ask for.
	SshKeys pulumi.StringArrayInput

	// SshSourceAddresses narrows which CIDRs may reach SSH. Optional: when SshKeys
	// is set and this is empty, port 22 is open to any source, which is what an
	// operator with a dynamic address needs. Set it when the addresses are stable
	// enough to be worth maintaining.
	SshSourceAddresses pulumi.StringArrayInput

	// Tags are applied to the droplet.
	Tags pulumi.StringArrayInput

	// Ipv6 enables IPv6 on the droplet. Defaults to true.
	Ipv6 pulumi.BoolInput

	// Monitoring enables the DigitalOcean monitoring agent. Defaults to true.
	Monitoring pulumi.BoolInput
}

// AuthServer is a Holochain auth server running on a DigitalOcean droplet, backed
// by a managed Valkey cluster and served over TLS by a Let's Encrypt certificate
// obtained on first boot.
type AuthServer struct {
	pulumi.ResourceState

	// Droplet is the underlying DigitalOcean droplet.
	Droplet *digitalocean.Droplet

	// Firewall restricts inbound traffic to the ports the service needs.
	Firewall *digitalocean.Firewall

	// Database is the managed Valkey cluster holding pending and approved keys.
	//
	// This state must outlive the droplet. DigitalOcean user data is immutable, so
	// any configuration change replaces the droplet; a containerised database
	// would take the approval records with it.
	Database *digitalocean.DatabaseCluster

	Ipv4Address pulumi.StringOutput
	Ipv6Address pulumi.StringOutput
	Hostname    pulumi.StringOutput

	// Url is the https:// endpoint the relay and agents connect to.
	Url pulumi.StringOutput

	// OpsUrl is the GitHub-authenticated console operators use to approve keys.
	OpsUrl pulumi.StringOutput
}

// New provisions an auth server.
//
// Changing any argument that affects cloud-init replaces the droplet: user data is
// immutable on DigitalOcean. The Valkey cluster is a separate resource and is not
// replaced with it, so approved keys survive.
func New(ctx *pulumi.Context, name string, args *Args, opts ...pulumi.ResourceOption) (*AuthServer, error) {
	if args == nil {
		return nil, errors.New("authserver.New: args is required")
	}
	for _, required := range []struct {
		name  string
		value pulumi.StringInput
	}{
		{"Hostname", args.Hostname},
		{"ContactEmail", args.ContactEmail},
		{"GithubClientId", args.GithubClientId},
		{"GithubClientSecret", args.GithubClientSecret},
		{"GithubOrg", args.GithubOrg},
		{"GithubTeam", args.GithubTeam},
		{"SessionSecret", args.SessionSecret},
		{"ApiTokens", args.ApiTokens},
	} {
		if required.value == nil {
			return nil, errors.New("authserver.New: " + required.name + " is required")
		}
	}
	component := &AuthServer{}
	if err := ctx.RegisterComponentResource(resourceType, name, component, opts...); err != nil {
		return nil, err
	}

	hostname := args.Hostname.ToStringOutput()
	region := inputs.StringOr(args.Region, defaultRegion)

	database, err := digitalocean.NewDatabaseCluster(ctx, name, &digitalocean.DatabaseClusterArgs{
		Name:      pulumi.String(name),
		Engine:    pulumi.String(databaseEngine),
		Version:   pulumi.String(defaultDatabaseVersion),
		Size:      inputs.StringOr(args.DatabaseSize, defaultDatabaseSize),
		Region:    region,
		NodeCount: pulumi.Int(1),
		// This database is a record, not a cache. Set explicitly so the guarantee
		// does not rest on a provider default we do not control.
		EvictionPolicy: pulumi.String(databaseEvictionPolicy),
	}, pulumi.Parent(component))
	if err != nil {
		return nil, err
	}

	userData := pulumi.All(
		hostname,
		args.ContactEmail.ToStringOutput(),
		inputs.StringOr(args.ContainerImage, DefaultContainerImage),
		args.GithubClientId.ToStringOutput(),
		args.GithubClientSecret.ToStringOutput(),
		args.GithubOrg.ToStringOutput(),
		args.GithubTeam.ToStringOutput(),
		args.SessionSecret.ToStringOutput(),
		args.ApiTokens.ToStringOutput(),
		// PrivateUri keeps Valkey traffic on DigitalOcean's private network rather
		// than routing it over the public internet.
		database.PrivateUri,
		inputs.IntOr(args.MaxPendingRequests, defaultMaxPendingRequests),
	).ApplyT(func(templateArgs []interface{}) (string, error) {
		return renderCloudInit(templateData{
			Hostname:           templateArgs[0].(string),
			ContactEmail:       templateArgs[1].(string),
			ContainerImage:     templateArgs[2].(string),
			GithubClientId:     templateArgs[3].(string),
			GithubClientSecret: templateArgs[4].(string),
			GithubOrg:          templateArgs[5].(string),
			GithubTeam:         templateArgs[6].(string),
			SessionSecret:      templateArgs[7].(string),
			ApiTokens:          templateArgs[8].(string),
			RedisUrl:           templateArgs[9].(string),
			MaxPendingRequests: templateArgs[10].(int),
		})
	}).(pulumi.StringOutput)

	droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
		Name:       pulumi.String(name),
		Image:      pulumi.String(defaultOsImage),
		Region:     region,
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

	// Without this the managed cluster accepts connections from anywhere. It is
	// password protected, but this is the database that decides who may join the
	// network, so it is worth closing to everything but its own droplet.
	if _, err := digitalocean.NewDatabaseFirewall(ctx, name, &digitalocean.DatabaseFirewallArgs{
		ClusterId: database.ID(),
		Rules: digitalocean.DatabaseFirewallRuleArray{
			&digitalocean.DatabaseFirewallRuleArgs{
				Type:  pulumi.String("droplet"),
				Value: droplet.ID().ToStringOutput(),
			},
		},
	}, pulumi.Parent(component)); err != nil {
		return nil, err
	}

	component.Droplet = droplet
	component.Firewall = firewall
	component.Database = database
	component.Ipv4Address = droplet.Ipv4Address
	component.Ipv6Address = droplet.Ipv6Address
	component.Hostname = hostname
	component.Url = hostname.ApplyT(func(h string) string { return "https://" + h }).(pulumi.StringOutput)
	component.OpsUrl = hostname.ApplyT(func(h string) string { return "https://" + h + "/ops/auth" }).(pulumi.StringOutput)

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"ipv4Address": component.Ipv4Address,
		"ipv6Address": component.Ipv6Address,
		"hostname":    component.Hostname,
		"url":         component.Url,
		"opsUrl":      component.OpsUrl,
	}); err != nil {
		return nil, err
	}

	return component, nil
}
