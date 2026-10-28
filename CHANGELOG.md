# Changelog

All notable changes to this project will be documented in this file.

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## \[[0.2.1](https://github.com/holochain/pulumi-network-services/compare/v0.2.0...v0.2.1)\] - 2026-08-07

### Bug Fixes

- Pin the Valkey eviction policy to noeviction by @ThetaSinner in [#10](https://github.com/holochain/pulumi-network-services/pull/10)
  - The auth server sets no TTL on anything it writes: an `auth:{id}` hash and its `state:{state}` set membership live until an operator deletes them. Under any allkeys policy Valkey would reclaim memory by dropping authorised keys, which revokes a peer's access with no error and no audit trail, and can leave the two structures disagreeing — an id still listed in a state set whose record is gone, which the ops console cannot show and which a fresh request cannot replace because the hash still exists.
  - Noeviction fails the write instead, which is a failure an operator can see.
  - This is DigitalOcean's default, so no deployed cluster changes behaviour and the update is applied in place rather than replacing the cluster. Setting it explicitly makes the guarantee ours rather than theirs, and puts it in the preview diff if it ever drifts.
  - Not exposed as an argument: there is one correct value, and the failure mode of the wrong one is silent.

## \[[0.2.0](https://github.com/holochain/pulumi-network-services/compare/v0.1.0...v0.2.0)\] - 2026-08-04

### Features

- Add NewAuthenticated to pair a relay with an auth server by @ThetaSinner in [#8](https://github.com/holochain/pulumi-network-services/pull/8)
  - Adds AuthHookServer to bootstraprelay.Args, rendered onto the command line only when set, and NewAuthenticated which provisions both hosts and wires the relay's hook from the auth server's own URL so the two cannot drift apart.
  - Arguments are flat with Relay and Auth prefixes rather than nested component arg structs, which would have carried two more Region fields that had to be ignored. Region is single and applies to both droplets and the database.
  - Also finishes the validation dedup started in the authserver change: bootstraprelay now uses internal/validate rather than its own copies.
- Add an authserver component by @ThetaSinner in [#7](https://github.com/holochain/pulumi-network-services/pull/7)
  - Provisions a Holochain auth server: a droplet running hc_auth_server, a managed Valkey cluster, and firewalls for both.
  - The database is managed rather than containerised because it holds the record of who may join the network, and DigitalOcean replaces the droplet on any cloud-init change. It is reachable only from its own droplet and connected over the private URI, so Valkey traffic never crosses the public internet.
  - Validation shared with bootstraprelay moves to internal/validate. The auth server renders most of its configuration into an environment file, where a line break would silently truncate every setting after it, so every value is checked.
- \[**BREAKING**\] Create a firewall and raise the default droplet size by @ThetaSinner in [#6](https://github.com/holochain/pulumi-network-services/pull/6)
  - The firewall is not optional. Its port matrix is the part a consumer would have to rediscover, and the two entries most easily missed both fail quietly: UDP 7842 degrades every peer onto relaying, and closing port 80 stops certbot renewing ninety days later. Egress stays open because DigitalOcean denies all egress when a firewall declares no outbound rules, which would strand cloud-init.
  - SSH is closed unless SshSourceAddresses says who may connect. Setting SshKeys without it is rejected before any resource is created, rather than silently opening port 22 or silently locking the operator out.
  - The default size moves to s-2vcpu-4gb, sized for production rather than for a cheap test. Kitsune2 scales its worker pool with the CPU count.

## \[[0.1.0](https://github.com/holochain/pulumi-network-services/commits/v0.1.0)\] - 2026-08-04

### Features

- Add BootstrapRelay component by @ThetaSinner
- Validate cloud-init inputs against YAML block escape by @ThetaSinner
- Add cloud-init template and renderer for bootstrap/relay by @ThetaSinner

### Miscellaneous Tasks

- Pin nix flake inputs with flake.lock by @ThetaSinner

### CI

- Add two-stage release workflows by @ThetaSinner in [#3](https://github.com/holochain/pulumi-network-services/pull/3)
  - Prepare computes the next version with git-cliff, writes CHANGELOG.md and opens a pull request labelled hra-release. Publish tags the merge commit and creates the GitHub release.
  - The tag is created last because a Go module version is immutable once the module proxy has served it. There is no version bump step, since a Go module's version is its tag rather than a manifest field, and no registry publish.
- Add vet, test and cloud-init lint by @ThetaSinner

### Documentation

- Note the QUIC address discovery port by @ThetaSinner
  - The bootstrap server binds UDP 7842 for QAD in production mode. Host networking means there is no port mapping to get wrong, but a firewall added in front of the droplet must cover UDP, not just TCP 443.
- Dual license under Apache-2.0 OR MIT by @ThetaSinner
- Add README with the versioning contract by @ThetaSinner

### Automated Changes

- *(deps)* Bump the gomod group across 1 directory with 2 updates by @dependabot[bot] in [#4](https://github.com/holochain/pulumi-network-services/pull/4)
- Update CODEOWNERS with shared content in [#2](https://github.com/holochain/pulumi-network-services/pull/2)
- Update dependabot.yml with shared content in [#1](https://github.com/holochain/pulumi-network-services/pull/1)

### Other Changes

- Initial commit by @holochain-release-automation2

### First-time Contributors

- @ThetaSinner made their first contribution in [#5](https://github.com/holochain/pulumi-network-services/pull/5)
- @dependabot[bot] made their first contribution in [#4](https://github.com/holochain/pulumi-network-services/pull/4)
- @ made their first contribution in [#2](https://github.com/holochain/pulumi-network-services/pull/2)
- @holochain-release-automation2 made their first contribution

<!-- generated by git-cliff -->
