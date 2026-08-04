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

#### Ports

The container runs with host networking, so the server binds the droplet's
interfaces directly and there is no port mapping to get wrong:

| Port | Protocol | Purpose |
| --- | --- | --- |
| 443 | TCP | Bootstrap and relay over HTTPS |
| 7842 | UDP | QUIC Address Discovery (QAD) |

`--production` enables the QAD listener, which is how peers learn their own public
address in order to attempt a direct connection. Losing it does not take the service
down — peers fall back to relaying — so it tends to fail quietly as degraded
connectivity rather than an outage.

The component creates a `digitalocean.Firewall` and attaches it. This is not
optional, because the port matrix is the part a consumer has to rediscover to get
right, and the two entries most easily missed both fail *quietly*:

| Port | Protocol | Why it must be open |
| --- | --- | --- |
| 443 | TCP | Bootstrap and relay |
| 7842 | UDP | QAD. A TCP-only rule set leaves the service looking healthy while forcing every peer onto relaying |
| 80 | TCP | Certbot. `certonly --standalone` uses HTTP-01, at renewal as well as first issue — closing it means certificates stop renewing 90 days later |
| — | ICMP | Path MTU Discovery. Blocking it black-holes large QUIC packets intermittently |

Egress is left open. A DigitalOcean firewall with no outbound rules denies *all*
egress, which would strand cloud-init before it finished — apt, the container
registry and the ACME challenge all need to get out.

#### SSH follows the keys

`SshKeys` is empty by default, so nobody can SSH in and port 22 stays closed.
Attaching keys opens it — from any source, unless you narrow it:

```go
SshKeys:            pulumi.StringArray{pulumi.String("aa:bb:cc:...")},
SshSourceAddresses: pulumi.StringArray{pulumi.String("203.0.113.0/24")}, // optional
```

Open-by-default is deliberate. Operators generally have dynamic addresses, and a
source list that must be rotated is one that goes stale or gets widened in a hurry
during an incident — neither of which is better than relying on the key pair, which
is the real control. Set `SshSourceAddresses` when your addresses are stable enough
to be worth maintaining.

Port 22 stays shut when no keys are attached, and that matters: DigitalOcean only
sets a root password when a droplet has no keys, so opening it in that case would
expose password authentication.

Filter `digitalocean.GetSshKeys` rather than passing all of it — every fingerprint
you pass gets root:

```go
keys, err := digitalocean.GetSshKeys(ctx, &digitalocean.GetSshKeysArgs{}, nil)
```

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

### `authserver`

A Holochain auth server, implementing the sbd authentication hook specification.
Pair it with `bootstraprelay` to require authentication before peers may use the
network.

```go
import "github.com/holochain/pulumi-network-services/authserver"

auth, err := authserver.New(ctx, "auth", &authserver.Args{
    Hostname:           pulumi.String("auth.example.org"),
    ContactEmail:       pulumi.String("ops@example.org"),
    GithubClientId:     cfg.RequireSecret("github-client-id"),
    GithubClientSecret: cfg.RequireSecret("github-client-secret"),
    GithubOrg:          pulumi.String("holochain"),
    GithubTeam:         pulumi.String("auth"),
    SessionSecret:      cfg.RequireSecret("session-secret"),
    ApiTokens:          cfg.RequireSecret("api-tokens"),
})
```

It creates a droplet, a **managed Valkey cluster**, and firewalls for both. It
exports `Url`, `OpsUrl`, `Hostname`, `Ipv4Address` and `Ipv6Address`.

#### The database is managed on purpose

Valkey holds the pending and approved keys — the record of who may join the
network. DigitalOcean user data is immutable, so any configuration change replaces
the droplet; a containerised database would take that record with it. The cluster
is a separate resource and survives droplet replacement.

It is reachable only from its own droplet, via a `digitalocean.DatabaseFirewall`,
and the server connects over the cluster's **private** URI so Valkey traffic never
crosses the public internet.

#### It is a public service

Unlike a typical internal dependency, this cannot hide behind a private network:
agents call `/now` and `/request-auth` directly, and the GitHub OAuth callback
redirects an operator's browser to `/ops`. It needs a public hostname and TLS.

That also means `/authenticate` cannot be restricted to the relay at the network
layer — it shares a listener with the public endpoints. Its protection is the
signature on the request, not the firewall.

#### Secrets reach the host through cloud-init

The GitHub client secret, session secret, API tokens and database URI are written
to `/opt/auth_srv/auth.env` (mode `0600`) by cloud-init. DigitalOcean exposes user
data through the droplet metadata service, so anything running on the host can read
them. That is inherent to configuring a droplet this way rather than specific to
this component, but it is worth knowing before you decide what else runs there.

## Versioning

The Go API is stable and easy to keep stable. The version of the service you deploy
is the thing that actually matters, so it gets an explicit contract:

| Bump | Means |
| --- | --- |
| **MAJOR** | A Go API break, or a change that replaces your resources *even if you pinned every argument* |
| **MINOR** | New components, new optional arguments, or a change to a default you can override — `DefaultContainerImage`, the droplet size, the region |
| **PATCH** | Fixes that change nothing you have deployed |

The line between MAJOR and MINOR is whether pinning saves you. That is why a change
to the droplet's OS image is MAJOR rather than MINOR: it is not exposed as an
argument, so it replaces the droplet of every consumer, including those who pinned
everything they could.

**Pin `ContainerImage` to a literal in production.** If you leave it unset you get
`DefaultContainerImage`, and a MINOR release can move that — which replaces your
droplet. Pinning to `bootstraprelay.DefaultContainerImage` is *not* pinning: that
constant moves with the library. Write the image reference out in full.

Releases that move `DefaultContainerImage` say so in their release notes, because
that is the change that replaces droplets for consumers who have not pinned.

Whatever you pin, `pulumi preview` shows the replacement before anything applies.

## Releasing

Two stages, matching the rest of the organisation. `CHANGELOG.md` is generated by
[git-cliff](https://git-cliff.org/) from commit messages, using the shared
[`pre-1.0-cliff.toml`](https://github.com/holochain/release-integration/blob/main/pre-1.0-cliff.toml),
so write [Conventional Commits](https://www.conventionalcommits.org/).

1. Run the **Prepare a release** workflow. It computes the next version from the
   commit history, writes `CHANGELOG.md`, and opens a pull request labelled
   `hra-release`. Pass `force_version` to override the computed bump.
2. Merge that pull request. **Publish release** then tags the merge commit and
   creates the GitHub release.

The tag is created last, and that ordering is not cosmetic. A Go module has no
manifest version — the version *is* the tag — and once `proxy.golang.org` has served
a tag, that version can never be changed. Tagging before the changelog merged would
publish a version that does not contain its own release notes, with no way to fix it
short of burning the next patch number.

This is why the shared `holochain/actions` release workflows are not used here: they
bump a version in `Cargo.toml` and publish to crates.io, neither of which has a Go
counterpart. Nothing needs publishing to a registry — the proxy picks up the tag.

### Releasing v2.0.0 or later

Go encodes the major version in the *import path* from v2 onwards, so a tag alone is
not enough and the release workflows do not handle this. Before preparing the first
v2 release:

1. Change the `module` line in `go.mod` to `github.com/holochain/pulumi-network-services/v2`.
2. Update imports within this repository to the `/v2` path.
3. Tell consumers to update their import paths — for Go this is a new module, so
   `go get` will not offer it as an upgrade of the old one.

Skipping this produces a `v2.0.0` tag that the module proxy will not serve as
`.../v2`, and consumers stay silently pinned to the newest v1.

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

## License

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE) or
  <https://www.apache.org/licenses/LICENSE-2.0>)
- MIT license ([LICENSE-MIT](LICENSE-MIT) or
  <https://opensource.org/licenses/MIT>)

at your option. In SPDX terms: `Apache-2.0 OR MIT`.

### Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted for
inclusion in the work by you, as defined in the Apache-2.0 license, shall be dual
licensed as above, without any additional terms or conditions.
