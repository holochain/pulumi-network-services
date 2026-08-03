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
	defaultRegion  = "fra1"
	defaultSize    = "s-2vcpu-2gb"
	defaultRustLog = "warn"

	// defaultOsImage is not exposed as an argument. Changing it replaces the
	// droplet even for a consumer who pinned every argument, so it is a MAJOR
	// release.
	defaultOsImage = "ubuntu-26-04-x64"
)
