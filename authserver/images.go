package authserver

// DefaultContainerImage is the hc_auth_server image deployed when
// Args.ContainerImage is not set.
//
// Changing this constant is a MINOR release. It replaces the droplet of every
// consumer who has not pinned Args.ContainerImage, so it always gets its own
// entry in the release notes.
const DefaultContainerImage = "ghcr.io/holochain/hc_auth_server:v0.1.6"

// Defaults applied when the corresponding Args field is unset.
const (
	defaultRegion = "fra1"

	// The auth server is a small web application: OAuth round-trips, Valkey
	// lookups and HTML rendering. It does not carry peer traffic, so it needs far
	// less than the relay does.
	defaultSize = "s-1vcpu-2gb"

	// The database holds pending and approved keys — a small key-value working
	// set, not a workload that scales with peer traffic.
	defaultDatabaseSize = "db-s-1vcpu-1gb"

	// Pinned rather than tracking latest: a major engine version is not something
	// a library should move under a consumer's live database.
	defaultDatabaseVersion = "8"

	// Caps how many unapproved requests are retained, so an unattended server
	// cannot be filled with junk faster than an operator can triage it.
	defaultMaxPendingRequests = 100

	// defaultOsImage is not exposed as an argument. Changing it replaces the
	// droplet even for a consumer who pinned every argument, so it is a MAJOR
	// release.
	defaultOsImage = "ubuntu-26-04-x64"

	// databaseEngine is Valkey, the Redis-compatible engine DigitalOcean offers.
	// The server speaks the Redis protocol and takes a redis:// URL.
	databaseEngine = "valkey"
)

// minSessionSecretBytes mirrors the server's own floor. Its env.example calls for
// a value "at least 64 bytes long for safety", and a short secret weakens every
// session cookie it signs.
const minSessionSecretBytes = 64

// Ports the auth server needs reachable. Unlike the relay there is no UDP
// listener: this host serves HTTPS only.
const (
	// portHttps carries the client API, the admin API and the ops console.
	portHttps = 443

	// portHttp is required by certbot. `certonly --standalone` uses the HTTP-01
	// challenge, which binds port 80 at every renewal, not only on first issue.
	portHttp = 80

	// portSsh is opened only when SSH keys are attached. DigitalOcean sets a root
	// password when a droplet has none, so opening it otherwise would expose
	// password authentication.
	portSsh = 22
)
