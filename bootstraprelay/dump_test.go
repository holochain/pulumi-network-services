package bootstraprelay

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDumpRenderedCloudInit writes rendered cloud-init to CLOUD_INIT_DUMP_DIR so CI
// can run `cloud-init schema` against it. The rendered output is deliberately not
// committed: a checked-in copy would either duplicate a downstream deployment's
// configuration or drift from the template.
//
// It is a no-op unless CLOUD_INIT_DUMP_DIR is set, so a normal `go test ./...`
// writes nothing.
func TestDumpRenderedCloudInit(t *testing.T) {
	dir := os.Getenv("CLOUD_INIT_DUMP_DIR")
	if dir == "" {
		t.Skip("CLOUD_INIT_DUMP_DIR is not set")
	}

	cases := map[string]templateData{
		"minimal": {
			Hostname:       "relay.example.org",
			ContactEmail:   "ops@example.org",
			ContainerImage: DefaultContainerImage,
			RustLog:        defaultRustLog,
		},
		"with-extra-args": {
			Hostname:       "relay.example.org",
			ContactEmail:   "ops@example.org",
			ContainerImage: DefaultContainerImage,
			RustLog:        "info,kitsune2=trace",
			ExtraArgs:      []string{"--extra-flag", "value"},
		},
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create dump dir: %v", err)
	}

	for name, data := range cases {
		got, err := renderCloudInit(data)
		if err != nil {
			t.Fatalf("renderCloudInit(%s): %v", name, err)
		}

		path := filepath.Join(dir, name+".cloud-init.yaml")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}
