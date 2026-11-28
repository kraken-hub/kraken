package util

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
)

func TestEnsureNetworks(t *testing.T) {
	t.Run("creates both networks when neither exists", func(t *testing.T) {
		var created []string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				created = append(created, opts.Name)
				return "id-" + opts.Name, nil
			},
		}
		tk := newTestKraken(t, rt)

		globalID, appID, err := EnsureNetworks(tk.Kraken, "myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if globalID != "id-kraken_net" {
			t.Errorf("globalID = %q, want id-kraken_net", globalID)
		}
		if appID != "id-kraken_net.myapp" {
			t.Errorf("appID = %q, want id-kraken_net.myapp", appID)
		}
		want := []string{"kraken_net", "kraken_net.myapp"}
		if len(created) != 2 || created[0] != want[0] || created[1] != want[1] {
			t.Errorf("created = %v, want %v", created, want)
		}
	})

	t.Run("skips creation when networks already exist", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return true, "existing-" + name, nil
			},
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				t.Fatalf("NetworkCreate should not be called for %q", opts.Name)
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)

		globalID, appID, err := EnsureNetworks(tk.Kraken, "myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if globalID != "existing-kraken_net" || appID != "existing-kraken_net.myapp" {
			t.Errorf("got (%q,%q)", globalID, appID)
		}
	})

	t.Run("kraken_net gets subnet and IP range from env", func(t *testing.T) {
		t.Setenv("KRAKEN_NET_SUBNET", "172.20.0.0/16")
		t.Setenv("KRAKEN_NET_IP_RANGE", "172.20.128.0/17")
		created := map[string]runtime.NetworkCreateOptions{}
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				created[opts.Name] = opts
				return "id", nil
			},
		}
		tk := newTestKraken(t, rt)

		if _, _, err := EnsureNetworks(tk.Kraken, "myapp"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g := created["kraken_net"]; g.Subnet != "172.20.0.0/16" || g.IPRange != "172.20.128.0/17" {
			t.Errorf("kraken_net subnet/range = %q/%q", g.Subnet, g.IPRange)
		}
		if a := created["kraken_net.myapp"]; a.Subnet != "" || a.IPRange != "" {
			t.Errorf("kraken_net.myapp subnet/range = %q/%q, want empty", a.Subnet, a.IPRange)
		}
	})

	t.Run("IP range without subnet errors before create", func(t *testing.T) {
		t.Setenv("KRAKEN_NET_SUBNET", "")
		t.Setenv("KRAKEN_NET_IP_RANGE", "172.20.128.0/17")
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				t.Fatalf("NetworkCreate should not be called for %q", opts.Name)
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)

		_, _, err := EnsureNetworks(tk.Kraken, "myapp")
		if err == nil || !strings.Contains(err.Error(), "KRAKEN_NET_IP_RANGE needs KRAKEN_NET_SUBNET") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("propagates NetworkExists error", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)

		if _, _, err := EnsureNetworks(tk.Kraken, "myapp"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("propagates NetworkCreate error", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				return "", context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)

		if _, _, err := EnsureNetworks(tk.Kraken, "myapp"); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsesKrakenNetworks(t *testing.T) {
	for mode, want := range map[string]bool{
		"":            true,
		"bridge":      true,
		"host":        false,
		"none":        false,
		"container:x": false,
		"service:x":   false,
	} {
		if got := UsesKrakenNetworks(mode); got != want {
			t.Errorf("UsesKrakenNetworks(%q) = %v, want %v", mode, got, want)
		}
	}
}

func TestPrepareNetworkMode(t *testing.T) {
	tests := []struct {
		name         string
		cfg          runtime.ContainerOptions
		wantMode     string
		wantPorts    bool
		droppedPorts bool
		wantErr      string
	}{
		{name: "default keeps ports", cfg: runtime.ContainerOptions{Ports: map[string]string{"80": "80/tcp"}}, wantMode: "", wantPorts: true},
		{name: "bridge keeps ports", cfg: runtime.ContainerOptions{NetworkMode: "bridge", Ports: map[string]string{"80": "80/tcp"}}, wantMode: "bridge", wantPorts: true},
		{name: "host drops ports", cfg: runtime.ContainerOptions{NetworkMode: "host", Ports: map[string]string{"80": "80/tcp"}}, wantMode: "host", droppedPorts: true},
		{name: "none without ports", cfg: runtime.ContainerOptions{NetworkMode: "none"}, wantMode: "none"},
		{name: "service resolved", cfg: runtime.ContainerOptions{NetworkMode: "service:db"}, wantMode: "container:db.myapp"},
		{name: "container literal", cfg: runtime.ContainerOptions{NetworkMode: "container:vpn"}, wantMode: "container:vpn"},
		{name: "container with conflicts", cfg: runtime.ContainerOptions{
			NetworkMode: "service:db",
			Ports:       map[string]string{"80": "80/tcp"},
			Hostname:    "h",
			DNS:         []string{"8.8.8.8"},
			ExtraHosts:  []string{"a:1.2.3.4"},
		}, wantErr: "ports, hostname, dns, extra_hosts"},
		{name: "networks with host", cfg: runtime.ContainerOptions{NetworkMode: "host", Networks: []runtime.NetworkAttachment{{Name: "shared"}}}, wantErr: "can't be combined with networks"},
		{name: "networks with bridge", cfg: runtime.ContainerOptions{NetworkMode: "bridge", Networks: []runtime.NetworkAttachment{{Name: "shared"}}}, wantMode: "bridge"},
		{name: "missing name", cfg: runtime.ContainerOptions{NetworkMode: "service:"}, wantErr: "missing name"},
		{name: "unsupported", cfg: runtime.ContainerOptions{NetworkMode: "mynet"}, wantErr: "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			dropped, err := prepareNetworkMode("myapp", &cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.NetworkMode != tt.wantMode {
				t.Errorf("NetworkMode = %q, want %q", cfg.NetworkMode, tt.wantMode)
			}
			if (len(cfg.Ports) > 0) != tt.wantPorts {
				t.Errorf("Ports = %#v, wantPorts %v", cfg.Ports, tt.wantPorts)
			}
			if dropped != tt.droppedPorts {
				t.Errorf("droppedPorts = %v, want %v", dropped, tt.droppedPorts)
			}
		})
	}
}

func appWithConfig(data string) *store.App {
	return &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: data}}}
}

func TestEnsureAppNetworks(t *testing.T) {
	t.Run("no networks section", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		got, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig("services:\n  api:\n    image: nginx\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %#v, want empty", got)
		}
	})

	t.Run("creates app networks with settings and labels", func(t *testing.T) {
		created := map[string]runtime.NetworkCreateOptions{}
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return false, "", nil },
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				created[opts.Name] = opts
				return "id", nil
			},
		}
		tk := newTestKraken(t, rt)
		got, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig(`networks:
  plain:
  backend:
    driver: bridge
    driver_opts:
      com.docker.network.bridge.name: br-backend
    internal: true
    labels:
      team: core
      kraken.app: spoofed
`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"plain": "kraken_net.myapp.plain", "backend": "kraken_net.myapp.backend"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
		plain := created["kraken_net.myapp.plain"]
		if plain.Driver != "" || plain.Internal || !reflect.DeepEqual(plain.Labels, map[string]string{LabelApp: "myapp", LabelNetwork: "plain"}) {
			t.Errorf("plain = %#v", plain)
		}
		backend := created["kraken_net.myapp.backend"]
		wantBackend := runtime.NetworkCreateOptions{
			Name:       "kraken_net.myapp.backend",
			Driver:     "bridge",
			DriverOpts: map[string]string{"com.docker.network.bridge.name": "br-backend"},
			Internal:   true,
			Labels:     map[string]string{LabelApp: "myapp", LabelNetwork: "backend", "team": "core"},
		}
		if !reflect.DeepEqual(backend, wantBackend) {
			t.Errorf("backend = %#v, want %#v", backend, wantBackend)
		}
	})

	t.Run("existing app network is not recreated", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return true, "id", nil },
			NetworkCreateFn: func(ctx context.Context, opts runtime.NetworkCreateOptions) (string, error) {
				t.Fatalf("NetworkCreate should not be called for %q", opts.Name)
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)
		if _, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig("networks:\n  backend:\n")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("external networks keep their name", func(t *testing.T) {
		var checked []string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				checked = append(checked, name)
				return true, "id", nil
			},
		}
		tk := newTestKraken(t, rt)
		got, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig(`networks:
  shared:
    external: true
  edge:
    external: true
    name: edge_net
  legacy:
    external:
      name: old_net
`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"shared": "shared", "edge": "edge_net", "legacy": "old_net"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("missing external network errors", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return false, "", nil },
		}
		tk := newTestKraken(t, rt)
		_, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig("networks:\n  shared:\n    external: true\n"))
		if err == nil || !strings.Contains(err.Error(), `external network "shared" does not exist`) {
			t.Fatalf("err = %v", err)
		}
	})

	for _, name := range []string{"default", "kraken_net"} {
		t.Run(name+" is reserved", func(t *testing.T) {
			tk := newTestKraken(t, &fakeRuntime{})
			_, err := EnsureAppNetworks(tk.Kraken, "myapp", appWithConfig("networks:\n  "+name+":\n"))
			if err == nil || !strings.Contains(err.Error(), "reserved") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestResolveNetworkAttachments(t *testing.T) {
	appNetworks := map[string]string{"backend": "kraken_net.myapp.backend", "shared": "shared"}

	t.Run("maps names, keeps aliases, drops duplicates", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{Name: "api.myapp", Networks: []runtime.NetworkAttachment{
			{Name: "backend", Aliases: []string{"api-internal"}},
			{Name: "shared"},
			{Name: "backend"},
		}}
		if err := ResolveNetworkAttachments(cfg, appNetworks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []runtime.NetworkAttachment{
			{Name: "kraken_net.myapp.backend", Aliases: []string{"api-internal"}},
			{Name: "shared"},
		}
		if !reflect.DeepEqual(cfg.Networks, want) {
			t.Errorf("got %#v, want %#v", cfg.Networks, want)
		}
	})

	t.Run("undeclared network errors", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{Name: "api.myapp", Networks: []runtime.NetworkAttachment{{Name: "nope"}}}
		err := ResolveNetworkAttachments(cfg, appNetworks)
		if err == nil || !strings.Contains(err.Error(), `network "nope" of `+"`api.myapp`"+` is not declared`) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("kraken_net needs no declaration, IPs are kept", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{Name: "api.myapp", Networks: []runtime.NetworkAttachment{
			{Name: "kraken_net", IPv4Address: "172.20.0.53"},
			{Name: "shared", IPv4Address: "10.5.0.10"},
		}}
		if err := ResolveNetworkAttachments(cfg, appNetworks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []runtime.NetworkAttachment{
			{Name: "kraken_net", IPv4Address: "172.20.0.53"},
			{Name: "shared", IPv4Address: "10.5.0.10"},
		}
		if !reflect.DeepEqual(cfg.Networks, want) {
			t.Errorf("got %#v, want %#v", cfg.Networks, want)
		}
	})

	for _, ip := range []string{"172.20.0", "fd00::53", "pier"} {
		t.Run("invalid ipv4_address "+ip+" errors", func(t *testing.T) {
			cfg := &runtime.ContainerOptions{Name: "api.myapp", Networks: []runtime.NetworkAttachment{{Name: "kraken_net", IPv4Address: ip}}}
			err := ResolveNetworkAttachments(cfg, appNetworks)
			if err == nil || !strings.Contains(err.Error(), "is not an IPv4 address") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestApplyRunIPv4Address(t *testing.T) {
	tests := []struct {
		name     string
		networks []runtime.NetworkAttachment
		ip       string
		want     []runtime.NetworkAttachment
	}{
		{
			name:     "service IPs are cleared",
			networks: []runtime.NetworkAttachment{{Name: "kraken_net", Aliases: []string{"dns.lh"}, IPv4Address: "172.20.0.53"}, {Name: "shared", IPv4Address: "10.5.0.10"}},
			want:     []runtime.NetworkAttachment{{Name: "kraken_net", Aliases: []string{"dns.lh"}}, {Name: "shared"}},
		},
		{
			name:     "ip replaces the kraken_net IP",
			networks: []runtime.NetworkAttachment{{Name: "kraken_net", IPv4Address: "172.20.0.53"}},
			ip:       "172.20.0.54",
			want:     []runtime.NetworkAttachment{{Name: "kraken_net", IPv4Address: "172.20.0.54"}},
		},
		{
			name:     "ip adds a kraken_net entry",
			networks: []runtime.NetworkAttachment{{Name: "shared", IPv4Address: "10.5.0.10"}},
			ip:       "172.20.0.54",
			want:     []runtime.NetworkAttachment{{Name: "shared"}, {Name: "kraken_net", IPv4Address: "172.20.0.54"}},
		},
		{
			name: "no networks, no ip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runtime.ContainerOptions{Networks: tt.networks}
			ApplyRunIPv4Address(cfg, tt.ip)
			if !reflect.DeepEqual(cfg.Networks, tt.want) {
				t.Errorf("got %#v, want %#v", cfg.Networks, tt.want)
			}
		})
	}
}

func TestIsIPv4(t *testing.T) {
	for s, want := range map[string]bool{"172.20.0.53": true, "0.0.0.0": true, "172.20.0": false, "fd00::53": false, "": false, "pier": false} {
		if got := IsIPv4(s); got != want {
			t.Errorf("IsIPv4(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestValidateExtraNetworks(t *testing.T) {
	rt := &fakeRuntime{
		NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
			return name == "shared" || name == "kraken_net.myapp.backend", "id", nil
		},
	}

	t.Run("existing networks by full name", func(t *testing.T) {
		tk := newTestKraken(t, rt)
		if err := ValidateExtraNetworks(tk.Kraken, "myapp", []string{"shared", "kraken_net.myapp.backend"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("app network key is not a network name", func(t *testing.T) {
		tk := newTestKraken(t, rt)
		err := ValidateExtraNetworks(tk.Kraken, "myapp", []string{"backend"})
		if err == nil || !strings.Contains(err.Error(), `network "backend" not found`) {
			t.Fatalf("err = %v", err)
		}
	})

	for _, name := range []string{"kraken_net", "kraken_net.myapp"} {
		t.Run(name+" is joined by default", func(t *testing.T) {
			tk := newTestKraken(t, rt)
			err := ValidateExtraNetworks(tk.Kraken, "myapp", []string{name})
			if err == nil || !strings.Contains(err.Error(), "joined by default") {
				t.Fatalf("err = %v", err)
			}
		})
	}

	for _, mode := range []string{"host", "none", "bridge", "service:db", "container:vpn"} {
		t.Run("mode "+mode+" is rejected", func(t *testing.T) {
			tk := newTestKraken(t, rt)
			err := ValidateExtraNetworks(tk.Kraken, "myapp", []string{mode})
			if err == nil || !strings.Contains(err.Error(), "use --network-mode") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAppendExtraNetworks(t *testing.T) {
	cfg := &runtime.ContainerOptions{Networks: []runtime.NetworkAttachment{
		{Name: "kraken_net.myapp.backend", Aliases: []string{"api-internal"}},
	}}
	AppendExtraNetworks(cfg, []string{"shared", "kraken_net.myapp.backend", "shared"})
	want := []runtime.NetworkAttachment{
		{Name: "kraken_net.myapp.backend", Aliases: []string{"api-internal"}},
		{Name: "shared"},
	}
	if !reflect.DeepEqual(cfg.Networks, want) {
		t.Errorf("got %#v, want %#v", cfg.Networks, want)
	}
}

func TestConnectNetworks(t *testing.T) {
	type connect struct {
		network string
		opts    runtime.NetworkConnectOptions
	}

	t.Run("kraken networks then extra networks", func(t *testing.T) {
		var got []connect
		rt := &fakeRuntime{
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, opts runtime.NetworkConnectOptions) error {
				got = append(got, connect{networkID, opts})
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "api.myapp", Networks: []runtime.NetworkAttachment{
			{Name: "shared", Aliases: []string{"api.lh"}, IPv4Address: "10.5.0.10"},
			{Name: "kraken_net.myapp.backend"},
		}}
		if err := ConnectNetworks(tk.Kraken, "myapp", "api", "cid", cfg, "global-id", "app-id"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []connect{
			{"global-id", runtime.NetworkConnectOptions{Aliases: []string{"api.myapp"}}},
			{"app-id", runtime.NetworkConnectOptions{Aliases: []string{"api"}}},
			{"shared", runtime.NetworkConnectOptions{Aliases: []string{"api.lh"}, IPv4Address: "10.5.0.10"}},
			{"kraken_net.myapp.backend", runtime.NetworkConnectOptions{}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("kraken_net entry sets aliases and IP of the global connection", func(t *testing.T) {
		var got []connect
		rt := &fakeRuntime{
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, opts runtime.NetworkConnectOptions) error {
				got = append(got, connect{networkID, opts})
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "dns.myapp", Networks: []runtime.NetworkAttachment{
			{Name: "kraken_net", Aliases: []string{"dns.lh"}, IPv4Address: "172.20.0.53"},
		}}
		if err := ConnectNetworks(tk.Kraken, "myapp", "dns", "cid", cfg, "global-id", "app-id"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []connect{
			{"global-id", runtime.NetworkConnectOptions{Aliases: []string{"dns.myapp", "dns.lh"}, IPv4Address: "172.20.0.53"}},
			{"app-id", runtime.NetworkConnectOptions{Aliases: []string{"dns"}}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("failed connect removes the container", func(t *testing.T) {
		var removed []string
		rt := &fakeRuntime{
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, opts runtime.NetworkConnectOptions) error {
				if opts.IPv4Address != "" {
					return errors.New("address already in use")
				}
				return nil
			},
			ContainerRemoveFn: func(ctx context.Context, id string, force, removeVolumes bool) error {
				removed = append(removed, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "dns.myapp", Networks: []runtime.NetworkAttachment{
			{Name: "kraken_net", IPv4Address: "172.20.0.53"},
		}}
		err := ConnectNetworks(tk.Kraken, "myapp", "dns", "cid", cfg, "global-id", "app-id")
		if err == nil || !strings.Contains(err.Error(), "address already in use") {
			t.Fatalf("err = %v", err)
		}
		if !reflect.DeepEqual(removed, []string{"cid"}) {
			t.Errorf("removed = %v, want [cid]", removed)
		}
	})

	t.Run("non-bridge mode connects nothing", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, opts runtime.NetworkConnectOptions) error {
				t.Fatalf("NetworkConnect should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := ConnectNetworks(tk.Kraken, "myapp", "api", "cid", &runtime.ContainerOptions{NetworkMode: "host"}, "g", "a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRemoveNetworks(t *testing.T) {
	t.Run("removes labelled networks and the app network once", func(t *testing.T) {
		var listedLabels map[string]string
		var removed []string
		rt := &fakeRuntime{
			NetworkListFn: func(ctx context.Context, labels map[string]string) ([]runtime.NetworkInfo, error) {
				listedLabels = labels
				return []runtime.NetworkInfo{
					{ID: "app-id", Name: "kraken_net.myapp"},
					{ID: "backend-id", Name: "kraken_net.myapp.backend"},
				}, nil
			},
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return name == "kraken_net.myapp", "app-id", nil
			},
			NetworkRemoveFn: func(ctx context.Context, id string) error {
				removed = append(removed, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		RemoveNetworks(tk.Kraken, "myapp")
		if !reflect.DeepEqual(listedLabels, map[string]string{LabelApp: "myapp"}) {
			t.Errorf("listed labels = %#v", listedLabels)
		}
		if !reflect.DeepEqual(removed, []string{"app-id", "backend-id"}) {
			t.Errorf("removed = %v", removed)
		}
	})

	t.Run("list error still removes the app network by name", func(t *testing.T) {
		var removed []string
		rt := &fakeRuntime{
			NetworkListFn: func(ctx context.Context, labels map[string]string) ([]runtime.NetworkInfo, error) {
				return nil, errors.New("boom")
			},
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return true, "app-id", nil },
			NetworkRemoveFn: func(ctx context.Context, id string) error {
				removed = append(removed, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		RemoveNetworks(tk.Kraken, "myapp")
		if !reflect.DeepEqual(removed, []string{"app-id"}) {
			t.Errorf("removed = %v", removed)
		}
		if !strings.Contains(tk.stdout(), "Warning listing networks") {
			t.Errorf("expected warning, stdout = %q", tk.stdout())
		}
	})

	t.Run("nothing to remove", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkListFn:   func(ctx context.Context, labels map[string]string) ([]runtime.NetworkInfo, error) { return nil, nil },
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return false, "", nil },
			NetworkRemoveFn: func(ctx context.Context, id string) error {
				t.Fatal("NetworkRemove should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		RemoveNetworks(tk.Kraken, "myapp")
	})
}
