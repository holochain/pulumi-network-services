package bootstraprelay

// DefaultContainerImage is the kitsune2_bootstrap_srv image deployed when
// Args.ContainerImage is not set.
//
// Changing this constant is a MINOR release. It replaces the droplet of every
// consumer who has not pinned Args.ContainerImage, so it always gets its own
// "Default image" entry in CHANGELOG.md.
const DefaultContainerImage = "ghcr.io/holochain/kitsune2_bootstrap_srv:v0.5.0"

// Defaults applied when the corresponding Args field is unset.
const (
	defaultRegion = "fra1"

	// defaultSize is sized for production rather than for a cheap test. Kitsune2
	// sets its worker thread count to four times the CPU count, so vCPUs are the
	// number that matters here. Override it for test deployments.
	defaultSize = "s-2vcpu-4gb"

	defaultRustLog = "warn"

	// defaultOsImage is not exposed as an argument. Changing it replaces the
	// droplet even for a consumer who pinned every argument, so it is a MAJOR
	// release.
	defaultOsImage = "ubuntu-26-04-x64"
)

// Ports the server needs reachable. These are not arguments: they are what the
// service actually binds, and a consumer guessing at them is the mistake the
// firewall exists to prevent.
const (
	// portHttps carries bootstrap and relay traffic.
	portHttps = 443

	// portQad is the QUIC Address Discovery listener, enabled by --production.
	// It is UDP. A TCP-only rule set leaves the service looking healthy while
	// silently forcing every peer onto relaying.
	portQad = 7842

	// portHttp is required by certbot. `certonly --standalone` uses the HTTP-01
	// challenge, which binds port 80 — not only on first boot but at every
	// renewal. Closing it yields a working server that stops renewing and fails
	// ninety days later. TLS-ALPN-01 is not an alternative here because the
	// relay owns 443.
	portHttp = 80

	portSsh = 22
)
