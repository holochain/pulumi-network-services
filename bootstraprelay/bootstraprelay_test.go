package bootstraprelay

import (
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks int

func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Copy()
	if args.TypeToken == "digitalocean:index/droplet:Droplet" {
		outputs["ipv4Address"] = resource.NewStringProperty("203.0.113.10")
		outputs["ipv6Address"] = resource.NewStringProperty("2001:db8::10")
	}
	return args.Name + "_id", outputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// run executes body inside a mocked Pulumi context. Assertions inside ApplyT run on
// another goroutine, so they must use t.Errorf, never t.Fatalf.
func run(t *testing.T, body func(ctx *pulumi.Context) error) {
	t.Helper()
	err := pulumi.RunErr(body, pulumi.WithMocks("network-services", "test", mocks(0)))
	if err != nil {
		t.Fatalf("pulumi program failed: %v", err)
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "dev-test-bootstrap2", &Args{
			Hostname:     pulumi.String("dev-test-bootstrap2.holochain.org"),
			ContactEmail: pulumi.String("contact@holochain.org"),
		})
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		wg.Add(1)
		pulumi.All(
			relay.Droplet.UserData,
			relay.Droplet.Region,
			relay.Droplet.Size,
			relay.Droplet.Image,
			relay.Droplet.Monitoring,
			relay.Droplet.Ipv6,
		).ApplyT(func(outputs []interface{}) error {
			defer wg.Done()

			userData, ok := outputs[0].(*string)
			if !ok || userData == nil {
				t.Errorf("droplet has no user data")
			} else {
				if !strings.Contains(*userData, "Image="+DefaultContainerImage+"\n") {
					t.Errorf("user data does not use DefaultContainerImage:\n%s", *userData)
				}
				if !strings.Contains(*userData, "Environment=RUST_LOG=warn\n") {
					t.Errorf("user data does not use the default RUST_LOG:\n%s", *userData)
				}
			}

			if got := outputs[1].(string); got != "fra1" {
				t.Errorf("region = %q, want fra1", got)
			}
			if got := outputs[2].(string); got != "s-2vcpu-2gb" {
				t.Errorf("size = %q, want s-2vcpu-2gb", got)
			}
			if got := outputs[3].(string); got != "ubuntu-26-04-x64" {
				t.Errorf("os image = %q, want ubuntu-26-04-x64", got)
			}
			if got, ok := outputs[4].(*bool); !ok || got == nil || !*got {
				t.Errorf("monitoring = %v, want true", outputs[4])
			}
			if got, ok := outputs[5].(*bool); !ok || got == nil || !*got {
				t.Errorf("ipv6 = %v, want true", outputs[5])
			}
			return nil
		})
		wg.Wait()
		return nil
	})
}

func TestNewHonoursExplicitArgs(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "custom", &Args{
			Hostname:       pulumi.String("relay.example.test"),
			ContactEmail:   pulumi.String("ops@example.test"),
			ContainerImage: pulumi.String("ghcr.io/holochain/kitsune2_bootstrap_srv:v0.9.9"),
			Region:         pulumi.String("nyc1"),
			Size:           pulumi.String("s-1vcpu-2gb"),
			RustLog:        pulumi.String("warn"),
			Monitoring:     pulumi.Bool(false),
			SshKeys:        pulumi.StringArray{pulumi.String("aa:bb:cc")},
			Tags:           pulumi.StringArray{pulumi.String("network-services")},
			ExtraArgs:      pulumi.StringArray{pulumi.String("--extra-flag")},
		})
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		wg.Add(1)
		pulumi.All(
			relay.Droplet.UserData,
			relay.Droplet.Region,
			relay.Droplet.Size,
			relay.Droplet.Monitoring,
			relay.Droplet.SshKeys,
			relay.Droplet.Tags,
			relay.Url,
		).ApplyT(func(outputs []interface{}) error {
			defer wg.Done()

			userData, ok := outputs[0].(*string)
			if !ok || userData == nil {
				t.Errorf("droplet has no user data")
			} else {
				for _, want := range []string{
					"Image=ghcr.io/holochain/kitsune2_bootstrap_srv:v0.9.9\n",
					"Environment=RUST_LOG=warn\n",
					`-d "relay.example.test" `,
					`-m "ops@example.test";`,
					"privkey.pem --extra-flag\n",
				} {
					if !strings.Contains(*userData, want) {
						t.Errorf("user data missing %q:\n%s", want, *userData)
					}
				}
			}

			if got := outputs[1].(string); got != "nyc1" {
				t.Errorf("region = %q, want nyc1", got)
			}
			if got := outputs[2].(string); got != "s-1vcpu-2gb" {
				t.Errorf("size = %q, want s-1vcpu-2gb", got)
			}
			if got, ok := outputs[3].(*bool); !ok || got == nil || *got {
				t.Errorf("monitoring = %v, want false", outputs[3])
			}
			if got := outputs[4].([]string); len(got) != 1 || got[0] != "aa:bb:cc" {
				t.Errorf("sshKeys = %v, want [aa:bb:cc]", got)
			}
			if got := outputs[5].([]string); len(got) != 1 || got[0] != "network-services" {
				t.Errorf("tags = %v, want [network-services]", got)
			}
			if got := outputs[6].(string); got != "https://relay.example.test" {
				t.Errorf("url = %q, want https://relay.example.test", got)
			}
			return nil
		})
		wg.Wait()
		return nil
	})
}

// A library must not attach SSH keys a consumer did not ask for.
func TestNewAttachesNoSshKeysByDefault(t *testing.T) {
	run(t, func(ctx *pulumi.Context) error {
		relay, err := New(ctx, "no-keys", &Args{
			Hostname:     pulumi.String("relay.example.test"),
			ContactEmail: pulumi.String("ops@example.test"),
		})
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		wg.Add(1)
		relay.Droplet.SshKeys.ApplyT(func(keys []string) error {
			defer wg.Done()
			if len(keys) != 0 {
				t.Errorf("sshKeys = %v, want none", keys)
			}
			return nil
		})
		wg.Wait()
		return nil
	})
}

func TestNewRejectsMissingRequiredArgs(t *testing.T) {
	tests := []struct {
		name string
		args *Args
		want string
	}{
		{name: "nil args", args: nil, want: "args is required"},
		{
			name: "no hostname",
			args: &Args{ContactEmail: pulumi.String("ops@example.test")},
			want: "Hostname is required",
		},
		{
			name: "no contact email",
			args: &Args{Hostname: pulumi.String("relay.example.test")},
			want: "ContactEmail is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := New(ctx, "invalid", tt.args)
				return err
			}, pulumi.WithMocks("network-services", "test", mocks(0)))

			if err == nil {
				t.Fatalf("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}
