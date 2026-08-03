# pulumi-network-services

Reusable [Pulumi](https://www.pulumi.com/) components for running Holochain network
services on DigitalOcean, written in Go.

These are the components Holochain uses to run its own infrastructure. DigitalOcean
is a hard dependency: if you need another provider, the source here is a reasonable
starting point to adapt.

## Install

```sh
go get github.com/holochain/pulumi-network-services
```

## Components

### `bootstraprelay`

A Kitsune2 bootstrap/relay server. From Kitsune2 0.5.0, peer discovery (bootstrap)
and connection establishment (relay) are a single service, so one component covers
both roles.

```go
import (
    "github.com/holochain/pulumi-network-services/bootstraprelay"
    "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

relay, err := bootstraprelay.New(ctx, "bootstrap-relay", &bootstraprelay.Args{
    Hostname:     pulumi.String("relay.example.org"),
    ContactEmail: pulumi.String("ops@example.org"),
    // A literal, so a release of this library cannot move it. See Versioning.
    ContainerImage: pulumi.String("ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0"),
})
```

The component creates a droplet and nothing else. It exports `Ipv4Address`,
`Ipv6Address`, `Hostname` and `Url`.

For a real deployment using this library — including DNS wiring — see
[holochain/network-services](https://github.com/holochain/network-services).

#### You provide the DNS

Pass the hostname you intend to use and create the A and AAAA records yourself, with
whatever DNS provider you use.

Certbot on the host retries with increasing backoff, but **not indefinitely**: 30
attempts over roughly 2.5 hours, after which the provisioning script exits non-zero
and the service does not start. Records created in the same `pulumi up` are fine so
long as they become resolvable inside that window.

#### You provide the SSH keys

`SshKeys` is empty by default, which means nobody can SSH into the droplet. Pass the
DigitalOcean fingerprints you want authorised:

```go
keys, err := digitalocean.GetSshKeys(ctx, &digitalocean.GetSshKeysArgs{}, nil)
```

Filter that result rather than passing all of it — every fingerprint you pass gets
root.

#### Updates replace the droplet

DigitalOcean user data is immutable, so any change that affects cloud-init —
including the container image — replaces the droplet.

That means a new IP address, so keep DNS TTLs low; records built from this
component's outputs update automatically. It also means a fresh Let's Encrypt
issuance, and Let's Encrypt permits **five duplicate certificates per week** for a
given hostname. Iterating repeatedly against one hostname will lock you out.

#### Resource options do not all reach the droplet

`pulumi.IgnoreChanges` passed to `New` applies to the *component*, and Pulumi does
not forward it to a component's children — so it has no effect on the droplet.

This matters if you feed `SshKeys` from `digitalocean.GetSshKeys`, which returns
every key on the account: `ssh_keys` forces replacement on DigitalOcean, so anyone
adding a key to your account replaces the droplet on your next `pulumi up`.

Prefer passing an explicit, stable list of fingerprints — then the input only changes
when you change it. If you need to ignore the field instead, use a transformation,
which *does* reach children:

```go
relay, err := bootstraprelay.New(ctx, "bootstrap-relay", args,
    pulumi.Transformations([]pulumi.ResourceTransformation{
        func(a *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
            if a.Type != "digitalocean:index/droplet:Droplet" {
                return nil
            }
            return &pulumi.ResourceTransformationResult{
                Props: a.Props,
                Opts:  append(a.Opts, pulumi.IgnoreChanges([]string{"sshKeys"})),
            }
        },
    }))
```

## Versioning

The Go API is stable and easy to keep stable. The version of the service you deploy
is the thing that actually matters, so it gets an explicit contract:

| Bump | Means |
| --- | --- |
| **MAJOR** | A Go API break, or a change that replaces your resources *even if you pinned every argument* |
| **MINOR** | New components, new optional arguments, or a `DefaultContainerImage` bump |
| **PATCH** | Fixes that change nothing you have deployed |

The line between MAJOR and MINOR is whether pinning saves you.

**Pin `ContainerImage` to a literal in production.** If you leave it unset you get
`DefaultContainerImage`, and a MINOR release can move that — which replaces your
droplet. Pinning to `bootstraprelay.DefaultContainerImage` is *not* pinning: that
constant moves with the library. Write the image reference out in full.

Releases that move `DefaultContainerImage` say so in their release notes, because
that is the change that replaces droplets for consumers who have not pinned.

Whatever you pin, `pulumi preview` shows the replacement before anything applies.

## Contributing

`nix develop` provides Go and `cloud-init`.

```sh
go test ./...
```

To lint the cloud-init the component renders, the way CI does:

```sh
CLOUD_INIT_DUMP_DIR=$(mktemp -d) go test ./bootstraprelay/ -run TestDumpRenderedCloudInit -v
cloud-init schema -c "$CLOUD_INIT_DUMP_DIR"/minimal.cloud-init.yaml
```

To develop against a consuming Pulumi program without publishing, use a Go
workspace from the consumer's directory:

```sh
go work init .
go work use ../pulumi-network-services
```

Keep `go.work` out of version control. Never commit a `replace` directive in a
consumer — that is how broken tags get published.
