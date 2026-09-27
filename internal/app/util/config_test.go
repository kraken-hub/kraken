package util

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

// parseNode parses a YAML document and returns its top-level content node
// plus the Ctx needed to resolve any aliases within it.
func parseNode(t *testing.T, doc string) (yaml.Node, *yaml.Ctx) {
	t.Helper()
	root, err := yaml.Load([]byte(doc))
	if err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}
	if root == nil {
		t.Fatalf("empty document: %q", doc)
	}
	ctx := yaml.NewCtx()
	ctx.Index(root)
	return root, ctx
}

// mappingValue returns the value node for key inside a mapping document.
func mappingValue(t *testing.T, doc, key string) (yaml.Node, *yaml.Ctx) {
	t.Helper()
	root, ctx := parseNode(t, doc)
	node := findMappingKey(ctx, root, key)
	if node == nil {
		t.Fatalf("key %q not found in %q", key, doc)
	}
	return node, ctx
}

func TestLoadServices(t *testing.T) {
	t.Run("no config snapshot errors", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp"}
		if _, err := LoadServices(appEntry); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid yaml errors", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: "foo: [1,2,3"}}}
		if _, err := LoadServices(appEntry); err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("no services key returns nil", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: "foo: bar\n"}}}
		got, err := LoadServices(appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("returns services in definition order", func(t *testing.T) {
		data := "services:\n  web:\n    image: nginx\n  db:\n    image: postgres\n  cache:\n    image: redis\n"
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: data}}}
		got, err := LoadServices(appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var names []string
		for _, s := range got {
			names = append(names, s.Name)
		}
		want := []string{"web", "db", "cache"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})
}

func TestLoadVolumesSection(t *testing.T) {
	t.Run("empty data returns empty map", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp"}
		got, _, err := LoadVolumesSection(appEntry)
		if err != nil || len(got) != 0 {
			t.Errorf("got %#v, %v", got, err)
		}
	})

	t.Run("invalid yaml errors", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: "foo: [1,2,3"}}}
		if _, _, err := LoadVolumesSection(appEntry); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("no volumes key returns empty map", func(t *testing.T) {
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: "services:\n  web:\n    image: nginx\n"}}}
		got, _, err := LoadVolumesSection(appEntry)
		if err != nil || len(got) != 0 {
			t.Errorf("got %#v, %v", got, err)
		}
	})

	t.Run("parses volume names to nodes", func(t *testing.T) {
		data := "volumes:\n  data:\n  cache:\n    driver: local\n"
		appEntry := &store.App{Key: "myapp", Value: store.AppValue{Config: store.AppConfig{Data: data}}}
		got, vctx, err := LoadVolumesSection(appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d entries, want 2", len(got))
		}
		if got["data"].Type() != yaml.NullType {
			t.Errorf("data node type = %v, want NullType", got["data"].Type())
		}
		if yaml.ScalarText(findMappingKey(vctx, got["cache"], "driver")) != "local" {
			t.Error("cache driver not parsed")
		}
	})
}

func TestFindMappingKey(t *testing.T) {
	node, ctx := parseNode(t, "a: 1\nb: 2\n")
	if v := findMappingKey(ctx, node, "b"); v == nil || yaml.ScalarText(v) != "2" {
		t.Errorf("findMappingKey(b) = %v", v)
	}
	if v := findMappingKey(ctx, node, "missing"); v != nil {
		t.Errorf("findMappingKey(missing) = %v, want nil", v)
	}
}

func TestNodeToContainerOptions(t *testing.T) {
	t.Run("basic fields and identity env injection", func(t *testing.T) {
		doc := "image: nginx\npull_policy: always\nworkdir: /srv\nhostname: myhost\nplatform: linux/amd64\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app/myapp", nil)

		if cfg.Name != "web.myapp" {
			t.Errorf("Name = %q", cfg.Name)
		}
		if cfg.Image != "nginx" {
			t.Errorf("Image = %q", cfg.Image)
		}
		if cfg.PullPolicy != "always" {
			t.Errorf("PullPolicy = %q", cfg.PullPolicy)
		}
		if cfg.WorkDir != "/srv" {
			t.Errorf("WorkDir = %q", cfg.WorkDir)
		}
		if cfg.Hostname != "myhost" {
			t.Errorf("Hostname = %q", cfg.Hostname)
		}
		if cfg.Platform != "linux/amd64" {
			t.Errorf("Platform = %q", cfg.Platform)
		}
		if cfg.Labels[LabelApp] != "myapp" || cfg.Labels[LabelService] != "web" {
			t.Errorf("Labels = %#v", cfg.Labels)
		}
		if cfg.Env["KRAKEN_APP"] != "myapp" || cfg.Env["KRAKEN_SERVICE"] != "web" {
			t.Errorf("Env = %#v", cfg.Env)
		}
	})

	t.Run("entrypoint and cmd scripts", func(t *testing.T) {
		doc := "entrypoint: /bin/entry.sh\ncmd:\n  - -c\n  - \"echo hi\"\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		if !reflect.DeepEqual(cfg.Entrypoint, []string{"/bin/entry.sh"}) {
			t.Errorf("Entrypoint = %#v", cfg.Entrypoint)
		}
		if !reflect.DeepEqual(cfg.Cmd, []string{"-c", "echo hi"}) {
			t.Errorf("Cmd = %#v", cfg.Cmd)
		}
	})

	t.Run("multiline cmd wrapped in shell", func(t *testing.T) {
		doc := "cmd: |\n  echo one\n  echo two\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		want := []string{"/bin/sh", "-c", "echo one\necho two\n"}
		if !reflect.DeepEqual(cfg.Cmd, want) {
			t.Errorf("Cmd = %#v, want %#v", cfg.Cmd, want)
		}
	})

	t.Run("env_file loaded then env overwrites", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "app.env"), []byte("A=fromfile\nB=fromfile\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		doc := "env_file: app.env\nenv:\n  B: fromenv\n  C: fromenv\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, dir, nil)
		if cfg.Env["A"] != "fromfile" {
			t.Errorf("A = %q, want fromfile", cfg.Env["A"])
		}
		if cfg.Env["B"] != "fromenv" {
			t.Errorf("B = %q, want fromenv (env overwrites env_file)", cfg.Env["B"])
		}
		if cfg.Env["C"] != "fromenv" {
			t.Errorf("C = %q, want fromenv", cfg.Env["C"])
		}
	})

	t.Run("volumes ports labels extra_hosts dns", func(t *testing.T) {
		doc := "volumes:\n  - data:/var/data\nports:\n  \"8080\": 80\nlabels:\n  team: infra\nextra_hosts:\n  - \"node{1..2}.lh:10.0.0.1\"\ndns:\n  - 8.8.8.8\ndns_search:\n  - example.com\ndns_opt:\n  - ndots:5\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)

		if len(cfg.Volumes) != 1 || cfg.Volumes[0].Target != "/var/data" {
			t.Errorf("Volumes = %#v", cfg.Volumes)
		}
		if cfg.Ports["8080"] != "80/tcp" {
			t.Errorf("Ports = %#v", cfg.Ports)
		}
		if cfg.UserLabels["team"] != "infra" {
			t.Errorf("UserLabels = %#v", cfg.UserLabels)
		}
		wantHosts := []string{"node1.lh:10.0.0.1", "node2.lh:10.0.0.1"}
		if !reflect.DeepEqual(cfg.ExtraHosts, wantHosts) {
			t.Errorf("ExtraHosts = %#v, want %#v", cfg.ExtraHosts, wantHosts)
		}
		if !reflect.DeepEqual(cfg.DNS, []string{"8.8.8.8"}) {
			t.Errorf("DNS = %#v", cfg.DNS)
		}
		if !reflect.DeepEqual(cfg.DNSSearch, []string{"example.com"}) {
			t.Errorf("DNSSearch = %#v", cfg.DNSSearch)
		}
		if !reflect.DeepEqual(cfg.DNSOptions, []string{"ndots:5"}) {
			t.Errorf("DNSOptions = %#v", cfg.DNSOptions)
		}
	})

	t.Run("volumes_from translates service names", func(t *testing.T) {
		doc := "volumes_from:\n  - db\n  - container:external-name\n  - cache:ro\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		want := []string{"db.myapp", "external-name", "cache.myapp:ro"}
		if !reflect.DeepEqual(cfg.VolumesFrom, want) {
			t.Errorf("VolumesFrom = %#v, want %#v", cfg.VolumesFrom, want)
		}
	})

	t.Run("logging section parsed", func(t *testing.T) {
		doc := "logging:\n  driver: json-file\n  options:\n    max-size: 10m\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		if cfg.Logging.Driver != "json-file" || cfg.Logging.Options["max-size"] != "10m" {
			t.Errorf("Logging = %#v", cfg.Logging)
		}
	})

	t.Run("restart mapping form", func(t *testing.T) {
		doc := "restart:\n  name: on-failure\n  max_retry: 5\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		if cfg.Restart != "on-failure" || cfg.RestartMaxRetry != 5 {
			t.Errorf("Restart = %q, RestartMaxRetry = %d", cfg.Restart, cfg.RestartMaxRetry)
		}
	})

	t.Run("build sets default image name", func(t *testing.T) {
		doc := "build: .\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		if cfg.Image != "kraken_myapp_web" {
			t.Errorf("Image = %q, want kraken_myapp_web", cfg.Image)
		}
		if cfg.Build == nil || cfg.Build.Context != "/app" {
			t.Errorf("Build = %#v", cfg.Build)
		}
	})

	t.Run("explicit image takes precedence over build default", func(t *testing.T) {
		doc := "image: custom:tag\nbuild: .\n"
		node, ctx := parseNode(t, doc)
		cfg := NodeToContainerOptions("myapp", "web", node, ctx, "/app", nil)
		if cfg.Image != "custom:tag" {
			t.Errorf("Image = %q, want custom:tag", cfg.Image)
		}
	})
}

func TestApplyVars(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())
	if err := store.SetVar("GLOBAL_VAR", "fromstore"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVar("OVERRIDDEN_VAR", "fromstore"); err != nil {
		t.Fatal(err)
	}

	cfg := &runtime.ContainerOptions{Env: map[string]string{"OVERRIDDEN_VAR": "fromyaml"}}
	ApplyVars(cfg)

	if cfg.Env["GLOBAL_VAR"] != "fromstore" {
		t.Errorf("GLOBAL_VAR = %q, want fromstore", cfg.Env["GLOBAL_VAR"])
	}
	if cfg.Env["OVERRIDDEN_VAR"] != "fromyaml" {
		t.Errorf("OVERRIDDEN_VAR = %q, want fromyaml (yaml takes precedence)", cfg.Env["OVERRIDDEN_VAR"])
	}
}

func TestApplyVarsNilEnv(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())
	if err := store.SetVar("ONLY_VAR", "value"); err != nil {
		t.Fatal(err)
	}
	cfg := &runtime.ContainerOptions{}
	ApplyVars(cfg)
	if cfg.Env["ONLY_VAR"] != "value" {
		t.Errorf("Env = %#v", cfg.Env)
	}
}

func TestWaitDuration(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want int64 // nanoseconds
	}{
		{"no wait key", "image: nginx\n", 0},
		{"empty wait value", "wait: \"\"\n", 0},
		{"wait false", "wait: \"false\"\n", 0},
		{"valid duration", "wait: 5s\n", int64(5_000_000_000)},
		{"invalid duration", "wait: notaduration\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, ctx := parseNode(t, tt.doc)
			got := WaitDuration(ctx, node)
			if got.Nanoseconds() != tt.want {
				t.Errorf("WaitDuration(%q) = %v, want %dns", tt.doc, got, tt.want)
			}
		})
	}
}

func TestParseScript(t *testing.T) {
	t.Run("single word scalar", func(t *testing.T) {
		node, ctx := mappingValue(t, "cmd: nginx\n", "cmd")
		got := parseScript(ctx, node)
		if !reflect.DeepEqual(got, []string{"nginx"}) {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("multi word scalar splits on fields", func(t *testing.T) {
		node, ctx := mappingValue(t, "cmd: echo hello world\n", "cmd")
		got := parseScript(ctx, node)
		if !reflect.DeepEqual(got, []string{"echo", "hello", "world"}) {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("multiline scalar wraps in shell", func(t *testing.T) {
		node, ctx := mappingValue(t, "cmd: |\n  echo a\n  echo b\n", "cmd")
		got := parseScript(ctx, node)
		want := []string{"/bin/sh", "-c", "echo a\necho b\n"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	t.Run("sequence form", func(t *testing.T) {
		node, ctx := mappingValue(t, "cmd:\n  - a\n  - b\n", "cmd")
		got := parseScript(ctx, node)
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("got %#v", got)
		}
	})
}

func TestParseBuild(t *testing.T) {
	t.Run("scalar sets context relative to appPath", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "build: ./docker\n", "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.Context != filepath.Join("/app", "docker") {
			t.Errorf("Context = %q", cfg.Build.Context)
		}
	})

	t.Run("mapping with context, file, args, tags, labels", func(t *testing.T) {
		doc := "build:\n  context: ./docker\n  file: Dockerfile.prod\n  args:\n    ENV: prod\n  target: builder\n  policy: always\n  no_cache: true\n  tags:\n    - myapp:latest\n  labels:\n    org: kraken\n  cache_from:\n    - myapp:cache\n  shm_size: 128mb\n  network: host\n"
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, doc, "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		b := cfg.Build
		if b.Context != filepath.Join("/app", "docker") {
			t.Errorf("Context = %q", b.Context)
		}
		if b.File != "Dockerfile.prod" {
			t.Errorf("File = %q", b.File)
		}
		if b.Args["ENV"] != "prod" {
			t.Errorf("Args = %#v", b.Args)
		}
		if b.Target != "builder" {
			t.Errorf("Target = %q", b.Target)
		}
		if b.Policy != "always" {
			t.Errorf("Policy = %q", b.Policy)
		}
		if !b.NoCache {
			t.Error("NoCache = false, want true")
		}
		if !reflect.DeepEqual(b.Tags, []string{"myapp:latest"}) {
			t.Errorf("Tags = %#v", b.Tags)
		}
		if b.Labels["org"] != "kraken" {
			t.Errorf("Labels = %#v", b.Labels)
		}
		if !reflect.DeepEqual(b.CacheFrom, []string{"myapp:cache"}) {
			t.Errorf("CacheFrom = %#v", b.CacheFrom)
		}
		if b.ShmSize != "128mb" {
			t.Errorf("ShmSize = %q", b.ShmSize)
		}
		if b.Network != "host" {
			t.Errorf("Network = %q", b.Network)
		}
	})

	t.Run("mapping without context defaults to appPath", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "build:\n  file: Dockerfile\n", "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.Context != "/app" {
			t.Errorf("Context = %q, want /app", cfg.Build.Context)
		}
	})

	t.Run("file_inline complements file path", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "build:\n  file: custom.Dockerfile\n  file_inline: |\n    FROM alpine\n", "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.File != "custom.Dockerfile" {
			t.Errorf("File = %q", cfg.Build.File)
		}
		if cfg.Build.InlineFile != "FROM alpine\n" {
			t.Errorf("InlineFile = %q", cfg.Build.InlineFile)
		}
	})

	t.Run("sequence with mapping items parses build files", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "build:\n  - Dockerfile: FROM alpine\n", "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.InlineFile != "FROM alpine" {
			t.Errorf("InlineFile = %q", cfg.Build.InlineFile)
		}
	})

	t.Run("sequence with scalar first item sets context", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "build:\n  - ./docker\n", "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.Context != filepath.Join("/app", "docker") {
			t.Errorf("Context = %q", cfg.Build.Context)
		}
	})

	t.Run("context as sequence with scalar and mapping items", func(t *testing.T) {
		doc := "build:\n  context:\n    - ./docker\n    - Dockerfile: FROM alpine\n"
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, doc, "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.Context != filepath.Join("/app", "docker") {
			t.Errorf("Context = %q", cfg.Build.Context)
		}
		if cfg.Build.InlineFile != "FROM alpine" {
			t.Errorf("InlineFile = %q", cfg.Build.InlineFile)
		}
	})

	t.Run("build args as sequence with bare key falls back to os env", func(t *testing.T) {
		t.Setenv("MY_BUILD_ARG", "from-os-env")
		doc := "build:\n  args:\n    - KEY=value\n    - MY_BUILD_ARG\n"
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, doc, "build")
		parseBuild(ctx, cfg, node, "/app", "myapp", "web")
		if cfg.Build.Args["KEY"] != "value" {
			t.Errorf("Args[KEY] = %q", cfg.Build.Args["KEY"])
		}
		if cfg.Build.Args["MY_BUILD_ARG"] != "from-os-env" {
			t.Errorf("Args[MY_BUILD_ARG] = %q", cfg.Build.Args["MY_BUILD_ARG"])
		}
	})
}

func TestParseRestart(t *testing.T) {
	t.Run("scalar form", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "restart: always\n", "restart")
		parseRestart(ctx, cfg, node)
		if cfg.Restart != "always" {
			t.Errorf("Restart = %q", cfg.Restart)
		}
	})
	t.Run("mapping form with only name", func(t *testing.T) {
		cfg := &runtime.ContainerOptions{}
		node, ctx := mappingValue(t, "restart:\n  name: on-failure\n", "restart")
		parseRestart(ctx, cfg, node)
		if cfg.Restart != "on-failure" || cfg.RestartMaxRetry != 0 {
			t.Errorf("Restart = %q, RestartMaxRetry = %d", cfg.Restart, cfg.RestartMaxRetry)
		}
	})
}

func TestParseLogging(t *testing.T) {
	t.Run("mapping form", func(t *testing.T) {
		node, ctx := mappingValue(t, "logging:\n  driver: json-file\n  options:\n    max-size: 10m\n", "logging")
		got := parseLogging(ctx, node)
		if got.Driver != "json-file" || got.Options["max-size"] != "10m" {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("non-mapping returns zero value", func(t *testing.T) {
		node, ctx := mappingValue(t, "logging: none\n", "logging")
		got := parseLogging(ctx, node)
		if got.Driver != "" || got.Options != nil {
			t.Errorf("got %#v, want zero value", got)
		}
	})
}

func TestParseVolumesFrom(t *testing.T) {
	got := parseVolumesFrom([]string{"db", "container:ext", "cache:ro"}, "myapp")
	want := []string{"db.myapp", "ext", "cache.myapp:ro"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestExpandExtraHosts(t *testing.T) {
	got := expandExtraHosts([]string{"node{1..2}.lh:10.0.0.1", "static.lh:10.0.0.2"})
	want := []string{"node1.lh:10.0.0.1", "node2.lh:10.0.0.1", "static.lh:10.0.0.2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestNodeToStringSlice(t *testing.T) {
	t.Run("scalar splits on fields", func(t *testing.T) {
		node, ctx := mappingValue(t, "x: a b c\n", "x")
		got := nodeToStringSlice(ctx, node)
		if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("sequence returns items", func(t *testing.T) {
		node, ctx := mappingValue(t, "x:\n  - a\n  - b\n", "x")
		got := nodeToStringSlice(ctx, node)
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("got %#v", got)
		}
	})
}

func TestParseBuildFileItems(t *testing.T) {
	t.Run("Dockerfile key sets InlineFile", func(t *testing.T) {
		b := &runtime.BuildOptions{}
		node, _ := mappingValue(t, "x:\n  Dockerfile: FROM alpine\n", "x")
		parseBuildFileItems(b, node.(*yaml.MappingNode))
		if b.InlineFile != "FROM alpine" {
			t.Errorf("InlineFile = %q", b.InlineFile)
		}
	})
	t.Run("other filename goes to InlineFiles", func(t *testing.T) {
		b := &runtime.BuildOptions{}
		node, _ := mappingValue(t, "x:\n  entrypoint.sh: \"#!/bin/sh\"\n", "x")
		parseBuildFileItems(b, node.(*yaml.MappingNode))
		if b.InlineFiles["entrypoint.sh"].Content != "#!/bin/sh" {
			t.Errorf("InlineFiles = %#v", b.InlineFiles)
		}
	})
	t.Run("mapping value with mode and content", func(t *testing.T) {
		b := &runtime.BuildOptions{}
		node, _ := mappingValue(t, "x:\n  run.sh:\n    mode: \"0755\"\n    content: \"echo hi\"\n", "x")
		parseBuildFileItems(b, node.(*yaml.MappingNode))
		bf := b.InlineFiles["run.sh"]
		if bf.Content != "echo hi" || bf.Mode != 0o755 {
			t.Errorf("got %#v", bf)
		}
	})
}

func TestParseBuildArgs(t *testing.T) {
	t.Run("mapping form", func(t *testing.T) {
		node, ctx := mappingValue(t, "args:\n  A: 1\n  B: 2\n", "args")
		got := parseBuildArgs(ctx, node)
		want := map[string]string{"A": "1", "B": "2"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	t.Run("sequence with equals and bare keys", func(t *testing.T) {
		t.Setenv("BARE_ARG", "hostval")
		node, ctx := mappingValue(t, "args:\n  - K=v\n  - BARE_ARG\n", "args")
		got := parseBuildArgs(ctx, node)
		want := map[string]string{"K": "v", "BARE_ARG": "hostval"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
}

func TestLabelsNodeToMap(t *testing.T) {
	t.Run("non-mapping returns empty", func(t *testing.T) {
		node, ctx := mappingValue(t, "labels: plain\n", "labels")
		got := labelsNodeToMap(ctx, node)
		if len(got) != 0 {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("scalar values pass through", func(t *testing.T) {
		node, ctx := mappingValue(t, "labels:\n  team: infra\n", "labels")
		got := labelsNodeToMap(ctx, node)
		if got["team"] != "infra" {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("nested value json encoded", func(t *testing.T) {
		node, ctx := mappingValue(t, "labels:\n  nested:\n    a: 1\n    b: 2\n", "labels")
		got := labelsNodeToMap(ctx, node)
		if got["nested"] != `{"a":1,"b":2}` {
			t.Errorf("nested = %q", got["nested"])
		}
	})
	t.Run("alias node resolved", func(t *testing.T) {
		doc := "base: &base\n  team: infra\nlabels: *base\n"
		node, ctx := mappingValue(t, doc, "labels")
		got := labelsNodeToMap(ctx, node)
		if got["team"] != "infra" {
			t.Errorf("got %#v", got)
		}
	})
}

func TestEnvNodeToMap(t *testing.T) {
	t.Run("mapping with null resolved from host env", func(t *testing.T) {
		t.Setenv("SESSION_SECRET", "topsecret")
		node, ctx := mappingValue(t, "env:\n  SESSION_SECRET:\n  FOO: bar\n", "env")
		got := envNodeToMap(ctx, node)
		want := map[string]string{"SESSION_SECRET": "topsecret", "FOO": "bar"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	t.Run("sequence with equals and bare entries", func(t *testing.T) {
		t.Setenv("BARE_ENV", "hostval")
		node, ctx := mappingValue(t, "env:\n  - FOO=bar\n  - BARE_ENV\n", "env")
		got := envNodeToMap(ctx, node)
		want := map[string]string{"FOO": "bar", "BARE_ENV": "hostval"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	t.Run("alias node resolved", func(t *testing.T) {
		doc := "base: &base\n  FOO: bar\nenv: *base\n"
		node, ctx := mappingValue(t, doc, "env")
		got := envNodeToMap(ctx, node)
		if got["FOO"] != "bar" {
			t.Errorf("got %#v", got)
		}
	})
}

func TestMappingNodeToMap(t *testing.T) {
	t.Run("mapping node", func(t *testing.T) {
		node, ctx := mappingValue(t, "x:\n  a: 1\n  b: 2\n", "x")
		got := mappingNodeToMap(ctx, node)
		want := map[string]string{"a": "1", "b": "2"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	t.Run("non-mapping returns empty", func(t *testing.T) {
		node, ctx := mappingValue(t, "x: plain\n", "x")
		got := mappingNodeToMap(ctx, node)
		if len(got) != 0 {
			t.Errorf("got %#v", got)
		}
	})
}

func TestParseEnvFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.env"), []byte("A=1\nB=1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "two.env"), []byte("B=2\nC=2\n"), 0o644)

	t.Run("scalar single path", func(t *testing.T) {
		node, ctx := mappingValue(t, "env_file: one.env\n", "env_file")
		got := parseEnvFiles(ctx, node, dir)
		want := map[string]string{"A": "1", "B": "1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence of paths later overrides earlier", func(t *testing.T) {
		node, ctx := mappingValue(t, "env_file:\n  - one.env\n  - two.env\n", "env_file")
		got := parseEnvFiles(ctx, node, dir)
		want := map[string]string{"A": "1", "B": "2", "C": "2"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("missing file skipped without error", func(t *testing.T) {
		node, ctx := mappingValue(t, "env_file: missing.env\n", "env_file")
		got := parseEnvFiles(ctx, node, dir)
		if len(got) != 0 {
			t.Errorf("got %#v, want empty", got)
		}
	})

	t.Run("absolute path used as-is", func(t *testing.T) {
		abs := filepath.Join(dir, "one.env")
		node, ctx := mappingValue(t, "env_file: "+abs+"\n", "env_file")
		got := parseEnvFiles(ctx, node, "/some/other/dir")
		if got["A"] != "1" {
			t.Errorf("got %#v", got)
		}
	})
}

func TestResolveVolumes(t *testing.T) {
	t.Run("mapping with null value uses container path as managed key", func(t *testing.T) {
		node, ctx := mappingValue(t, "volumes:\n  /var/data:\n", "volumes")
		got := resolveVolumes(ctx, node, "/app", "myapp", "web", nil)
		want := []runtime.VolumeMount{{Source: ServiceVolumeName("myapp", "web", "/var/data"), Target: "/var/data"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("mapping with explicit target", func(t *testing.T) {
		node, ctx := mappingValue(t, "volumes:\n  data: /var/data\n", "volumes")
		got := resolveVolumes(ctx, node, "/app", "myapp", "web", map[string]string{"data": "kraken_vol.myapp.data"})
		want := []runtime.VolumeMount{{Source: "kraken_vol.myapp.data", Target: "/var/data"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence single path form", func(t *testing.T) {
		node, ctx := mappingValue(t, "volumes:\n  - /var/data\n", "volumes")
		got := resolveVolumes(ctx, node, "/app", "myapp", "web", nil)
		want := []runtime.VolumeMount{{Source: ServiceVolumeName("myapp", "web", "/var/data"), Target: "/var/data"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence source:target form", func(t *testing.T) {
		node, ctx := mappingValue(t, "volumes:\n  - ./data:/var/data\n", "volumes")
		got := resolveVolumes(ctx, node, "/app", "myapp", "web", nil)
		want := []runtime.VolumeMount{{Source: filepath.Join("/app", "data"), Target: "/var/data"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence skips empty entries", func(t *testing.T) {
		node, ctx := mappingValue(t, "volumes:\n  - \"\"\n  - /var/data\n", "volumes")
		got := resolveVolumes(ctx, node, "/app", "myapp", "web", nil)
		if len(got) != 1 {
			t.Errorf("got %#v, want 1 entry", got)
		}
	})
}

func TestResolveVolumeSrc(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		managed map[string]string
		want    string
	}{
		{"empty source", "", nil, ""},
		{"absolute path", "/host/data", nil, "/host/data"},
		{"dot slash prefix", "./data", nil, filepath.Join("/app", "data")},
		{"dot dot slash prefix", "../data", nil, filepath.Join("/app", "../data")},
		{"dot alone", ".", nil, filepath.Join("/app", ".")},
		{"relative with slash", "sub/data", nil, filepath.Join("/app", "sub/data")},
		{"managed volume key", "data", map[string]string{"data": "kraken_vol.myapp.data"}, "kraken_vol.myapp.data"},
		{"unmanaged named volume falls back to service volume", "cache", nil, ServiceVolumeName("myapp", "web", "cache")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveVolumeSrc(tt.source, "/app", "myapp", "web", tt.managed)
			if got != tt.want {
				t.Errorf("resolveVolumeSrc(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestParsePorts(t *testing.T) {
	t.Run("mapping form adds default tcp proto", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  \"8080\": \"80\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"8080": "80/tcp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("mapping form preserves explicit proto", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  \"53\": \"53/udp\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"53": "53/udp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence containerPort only", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  - \"80\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"0:80": "80/tcp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence hostPort:containerPort", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  - \"8080:80\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"8080": "80/tcp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence ip:hostPort:containerPort", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  - \"127.0.0.1:8080:80\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"127.0.0.1:8080": "80/tcp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("sequence with explicit protocol suffix", func(t *testing.T) {
		node, ctx := mappingValue(t, "ports:\n  - \"53:53/udp\"\n", "ports")
		got := parsePorts(ctx, node)
		want := map[string]string{"53": "53/udp"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
}
