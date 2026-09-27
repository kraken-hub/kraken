package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

func tagAsJSFunction(value yaml.Node) *yaml.TagNode {
	return yaml.NewTag(value, "!!js/function")
}

// --- scale expansion ---

func TestPrepareYaml_ScaleExpansion_Basic(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    scale: 3
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web1": map[string]interface{}{"image": "nginx"},
			"web2": map[string]interface{}{"image": "nginx"},
			"web3": map[string]interface{}{"image": "nginx"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_ScaleExpansion_ScaleOneRemovesKeyWithoutExpanding(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    scale: 1
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{"image": "nginx"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_ScaleExpansion_InvalidValueRemovesKeyWithoutExpanding(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    scale: abc
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{"image": "nginx"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// scale: false is not applicable (no expansion), and the scale key is
// always stripped from the output regardless of its value, consistent with
// the scale<=1/invalid branches.
func TestPrepareYaml_ScaleExpansion_FalseValueRemovesKeyWithoutExpanding(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    scale: false
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{"image": "nginx"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_ScaleExpansion_LeavesOtherServicesUntouched(t *testing.T) {
	doc := `
services:
  db:
    image: postgres
  web:
    image: nginx
    scale: 2
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"db":   map[string]interface{}{"image": "postgres"},
			"web1": map[string]interface{}{"image": "nginx"},
			"web2": map[string]interface{}{"image": "nginx"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// --- ~env sections integration (top-level toggle + service scale via env) ---

func TestPrepareYaml_EnvSection_TopLevelMergeThenScaleExpansion(t *testing.T) {
	doc := `
services:
  app:
    image: myapp
~prod:
  services:
    worker:
      image: workerimg
      scale: 3
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, "prod"); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"app":     map[string]interface{}{"image": "myapp"},
			"worker1": map[string]interface{}{"image": "workerimg"},
			"worker2": map[string]interface{}{"image": "workerimg"},
			"worker3": map[string]interface{}{"image": "workerimg"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_EnvSection_NonMatchingEnvDropsService(t *testing.T) {
	doc := `
services:
  app:
    image: myapp
~prod:
  services:
    worker:
      image: workerimg
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, "dev"); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"app": map[string]interface{}{"image": "myapp"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_NoServicesKeyIsANoop(t *testing.T) {
	doc := `
volumes:
  data:
    driver: local
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, "dev"); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"volumes": map[string]interface{}{
			"data": map[string]interface{}{"driver": "local"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// --- service-level `env:` sequence -> mapping conversion ---

func TestPrepareYaml_ServiceEnvSequenceConvertedToMapping(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    env:
      - FOO=bar
      - BAZ=qux
      - FLAG
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{
				"image": "nginx",
				"env": map[string]interface{}{
					"FOO":  "bar",
					"BAZ":  "qux",
					"FLAG": nil,
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_ServiceEnvMappingLeftAsIs(t *testing.T) {
	doc := `
services:
  web:
    image: nginx
    env:
      FOO: bar
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{
				"image": "nginx",
				"env":   map[string]interface{}{"FOO": "bar"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// --- extend (same-file) ---

func TestPrepareYaml_ExtendSimple_KeyFilterAvoidsAliasingIssue(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
    labels:
      a: "1"
  web:
    extend: base&env
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"services": map[string]interface{}{
			"base": map[string]interface{}{
				"image": "base-image",
				"env":   map[string]interface{}{"FOO": "bar"},
				"labels": map[string]interface{}{
					"a": "1",
				},
			},
			"web": map[string]interface{}{
				"image": "web-image",
				"env":   map[string]interface{}{"FOO": "bar"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareYaml_ExtendExcludeFilterBang(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
    labels:
      a: "1"
  web:
    extend: base!labels
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{
		"image": "web-image",
		"env":   map[string]interface{}{"FOO": "bar"},
	}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

func TestPrepareYaml_ExtendMappingForm_IncludeType(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
    labels:
      a: "1"
  web:
    extend:
      service: base
      type: include
      keys: [labels]
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{
		"image":  "web-image",
		"labels": map[string]interface{}{"a": "1"},
	}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

func TestPrepareYaml_ExtendMappingForm_ExcludeType(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
    labels:
      a: "1"
  web:
    extend:
      service: base
      type: exclude
      keys: [labels]
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{
		"image": "web-image",
		"env":   map[string]interface{}{"FOO": "bar"},
	}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

func TestPrepareYaml_Extend_StripsPortsAndScaleFromBase(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    labels:
      a: "1"
    ports:
      - "80:80"
    scale: 3
  web:
    extend: base&labels
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	if _, ok := web["ports"]; ok {
		t.Fatalf("expected ports not inherited via extend (same-file), got %#v", web)
	}
	if _, ok := web["scale"]; ok {
		t.Fatalf("expected scale not inherited via extend (same-file), got %#v", web)
	}
}

func TestPrepareYaml_ExtendJSFunction_WithFilter(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
    labels:
      a: "1"
  web:
    extend:
      service: base
      type: include
      keys: [env, image]
      fn: !!js/function |
        ({ local, service }) => ({ ...service, ...local })
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})

	wantBase := map[string]interface{}{
		"image":  "base-image",
		"env":    map[string]interface{}{"FOO": "bar"},
		"labels": map[string]interface{}{"a": "1"},
	}
	if !reflect.DeepEqual(services["base"], wantBase) {
		t.Fatalf("expected base service untouched, got %#v", services["base"])
	}

	wantWeb := map[string]interface{}{
		"image": "web-image",
		"env":   map[string]interface{}{"FOO": "bar"},
	}
	if !reflect.DeepEqual(services["web"], wantWeb) {
		t.Fatalf("got web=%#v, want %#v", services["web"], wantWeb)
	}
}

// TestPrepareYaml_ExtendWithoutFilter_PreservesBaseService verifies that
// `extend` (same-file) without an include/exclude key filter does not mutate
// the base service: ExtendYamlService always copies the base node via
// filterNodeByKeys before merging, so the base and the extending service end
// up as independent nodes.
func TestPrepareYaml_ExtendWithoutFilter_PreservesBaseService(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
  web:
    extend: base
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})

	wantBase := map[string]interface{}{
		"image": "base-image",
		"env":   map[string]interface{}{"FOO": "bar"},
	}
	wantWeb := map[string]interface{}{
		"image": "web-image",
		"env":   map[string]interface{}{"FOO": "bar"},
	}
	if !reflect.DeepEqual(services["base"], wantBase) {
		t.Fatalf("got base=%#v, want %#v", services["base"], wantBase)
	}
	if !reflect.DeepEqual(services["web"], wantWeb) {
		t.Fatalf("got web=%#v, want %#v", services["web"], wantWeb)
	}
}

// TestPrepareYaml_TwoServicesExtendingSameBase_AreIndependent verifies that
// two services extending the same unfiltered base each get their own copy,
// so neither extender leaks into the other or into the base.
func TestPrepareYaml_TwoServicesExtendingSameBase_AreIndependent(t *testing.T) {
	doc := `
services:
  base:
    image: base-image
    env:
      FOO: bar
  web1:
    extend: base
    image: web1-image
  web2:
    extend: base
    image: web2-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})

	want := map[string]string{"base": "base-image", "web1": "web1-image", "web2": "web2-image"}
	for name, wantImage := range want {
		img := services[name].(map[string]interface{})["image"]
		if img != wantImage {
			t.Fatalf("expected %s.image == %s, got %v", name, wantImage, img)
		}
	}
}

// --- extends (external file) ---

func writeTempYaml(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp yaml: %v", err)
	}
	return path
}

func TestPrepareYaml_Extends_InheritsPortsAndOverridesImage(t *testing.T) {
	dir := t.TempDir()
	basePath := writeTempYaml(t, dir, "base.yaml", `
services:
  php:
    image: php-base
    ports:
      - "9000:9000"
    labels:
      tier: backend
`)

	doc := `
services:
  web:
    extends:
      file: ` + basePath + `
      service: php
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{
		"image":  "web-image",
		"ports":  []interface{}{"9000:9000"},
		"labels": map[string]interface{}{"tier": "backend"},
	}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

func TestPrepareYaml_Extends_JSFunction(t *testing.T) {
	dir := t.TempDir()
	basePath := writeTempYaml(t, dir, "base.yaml", `
services:
  php:
    image: php-base
    ports:
      - "9000:9000"
`)

	doc := `
services:
  web:
    extends:
      file: ` + basePath + `
      service: php
      fn: !!js/function |
        ({ local, service }) => ({ ...service, ...local, merged: true })
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{
		"image":  "web-image",
		"ports":  []interface{}{"9000:9000"},
		"merged": true,
	}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

// TestPrepareYaml_Extends_ScaledBaseServiceReturnsError verifies that
// referencing a scaled base service via `extends` returns a descriptive
// error instead of panicking. LoadYamlFile applies PrepareYaml (including
// scale expansion) to the extended file *before* the extending service's
// `service: <name>` lookup happens, so a scaled base service no longer
// exists under its original name (it becomes name1, name2, ...) by the time
// ExtendYamlService looks it up.
func TestPrepareYaml_Extends_ScaledBaseServiceReturnsError(t *testing.T) {
	dir := t.TempDir()
	basePath := writeTempYaml(t, dir, "base.yaml", `
services:
  php:
    image: php-base
    scale: 2
`)

	doc := `
services:
  web:
    extends:
      file: ` + basePath + `
      service: php
    image: web-image
`
	node := mustLoadNode(t, doc)

	err := PrepareYaml(node, "")
	if err == nil {
		t.Fatalf("expected an error when extends references a scaled base service, got nil")
	}
}

// TestPrepareYaml_Extends_ScaledBaseServiceByExpandedName verifies the
// documented workaround: referencing a scaled base service's expanded
// instance name (e.g. "php1") works, since that name legitimately exists
// in the base file's services after its own scale expansion runs.
func TestPrepareYaml_Extends_ScaledBaseServiceByExpandedName(t *testing.T) {
	dir := t.TempDir()
	basePath := writeTempYaml(t, dir, "base.yaml", `
services:
  php:
    image: php-base
    scale: 2
`)

	doc := `
services:
  web:
    extends:
      file: ` + basePath + `
      service: php1
    image: web-image
`
	node := mustLoadNode(t, doc)
	if err := PrepareYaml(node, ""); err != nil {
		t.Fatalf("PrepareYaml failed: %v", err)
	}

	got := dumpToValue(t, node)
	services := got.(map[string]interface{})["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	want := map[string]interface{}{"image": "web-image"}
	if !reflect.DeepEqual(web, want) {
		t.Fatalf("got web=%#v, want %#v", web, want)
	}
}

// --- ExtendYamlService (direct) ---

func buildServicesNode(pairs ...*yaml.MappingValueNode) *yaml.MappingNode {
	return mapping(pairs...)
}

func TestExtendYamlService_Direct_MergeOverridesAndAppends(t *testing.T) {
	base := mapping(strPair("image", "base-image"), pair(str("env"), mapping(strPair("FOO", "bar"))))
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	if err := ExtendYamlService(services, services, nil, 1, "base", nil, nil, true); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	_, mergedWeb := yaml.FindValue(yaml.NewCtx(), services, "web")
	mergedWebMap := mergedWeb.(*yaml.MappingNode)
	got := map[string]string{}
	envKeys := map[string]string{}
	for _, mv := range mergedWebMap.Values {
		key := yaml.ScalarText(mv.Key)
		if key == "env" {
			for _, emv := range mv.Value.(*yaml.MappingNode).Values {
				envKeys[yaml.ScalarText(emv.Key)] = yaml.ScalarText(emv.Value)
			}
			continue
		}
		got[key] = yaml.ScalarText(mv.Value)
	}

	if got["image"] != "web-image" {
		t.Fatalf("expected image overridden to web-image, got %#v", got)
	}
	if envKeys["FOO"] != "bar" {
		t.Fatalf("expected env inherited from base, got %#v", envKeys)
	}
}

func TestExtendYamlService_Direct_NoFilterKeepsDistinctNodes(t *testing.T) {
	base := mapping(strPair("image", "base-image"))
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	if err := ExtendYamlService(services, services, nil, 1, "base", nil, nil, true); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	_, baseAfter := yaml.FindValue(yaml.NewCtx(), services, "base")
	_, webAfter := yaml.FindValue(yaml.NewCtx(), services, "web")

	if baseAfter == webAfter {
		t.Fatalf("expected base and web to be distinct nodes even without a key filter")
	}
}

func TestExtendYamlService_Direct_WithIncludeFilterAvoidsAliasing(t *testing.T) {
	base := mapping(strPair("image", "base-image"), pair(str("env"), mapping(strPair("FOO", "bar"))))
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	if err := ExtendYamlService(services, services, nil, 1, "base", []string{"env"}, nil, true); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	ctx := yaml.NewCtx()
	_, baseAfter := yaml.FindValue(ctx, services, "base")
	_, webAfter := yaml.FindValue(ctx, services, "web")

	if baseAfter == webAfter {
		t.Fatalf("expected base and web to be distinct nodes when a key filter is used")
	}
	if baseAfter != yaml.Node(base) {
		t.Fatalf("expected original base node to remain unchanged in place")
	}
	_, baseImage := yaml.FindValue(ctx, baseAfter, "image")
	if yaml.ScalarText(baseImage) != "base-image" {
		t.Fatalf("expected base image untouched, got %q", yaml.ScalarText(baseImage))
	}
}

func TestExtendYamlService_Direct_StripsPortsAndScaleWhenRequested(t *testing.T) {
	base := mapping(
		strPair("image", "base-image"),
		pair(str("ports"), sequence(str("80:80"))),
		strPair("scale", "3"),
	)
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	if err := ExtendYamlService(services, services, nil, 1, "base", []string{"image", "ports", "scale"}, nil, true); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	ctx := yaml.NewCtx()
	_, webAfter := yaml.FindValue(ctx, services, "web")
	if _, n := yaml.FindValue(ctx, webAfter, "ports"); n != nil {
		t.Fatalf("expected ports stripped when stripPortsScale=true")
	}
	if _, n := yaml.FindValue(ctx, webAfter, "scale"); n != nil {
		t.Fatalf("expected scale stripped when stripPortsScale=true")
	}
}

func TestExtendYamlService_Direct_KeepsPortsAndScaleWhenNotStripped(t *testing.T) {
	base := mapping(
		strPair("image", "base-image"),
		pair(str("ports"), sequence(str("80:80"))),
	)
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	if err := ExtendYamlService(services, services, nil, 1, "base", []string{"image", "ports"}, nil, false); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	ctx := yaml.NewCtx()
	_, webAfter := yaml.FindValue(ctx, services, "web")
	if _, n := yaml.FindValue(ctx, webAfter, "ports"); n == nil {
		t.Fatalf("expected ports retained when stripPortsScale=false")
	}
}

func TestExtendYamlService_Direct_JSFunctionMerge(t *testing.T) {
	base := mapping(strPair("image", "base-image"), pair(str("env"), mapping(strPair("FOO", "bar"))))
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	fn := yaml.NewString(`({ local, service }) => ({ ...service, ...local, injected: true })`)

	if err := ExtendYamlService(services, services, tagAsJSFunction(fn), 1, "base", []string{"image", "env"}, nil, true); err != nil {
		t.Fatalf("ExtendYamlService failed: %v", err)
	}

	ctx := yaml.NewCtx()
	_, webAfter := yaml.FindValue(ctx, services, "web")
	_, injected := yaml.FindValue(ctx, webAfter, "injected")
	if injected == nil || yaml.ScalarText(injected) != "true" {
		t.Fatalf("expected injected=true from JS merge function, got %#v", injected)
	}
	_, image := yaml.FindValue(ctx, webAfter, "image")
	if yaml.ScalarText(image) != "web-image" {
		t.Fatalf("expected local image to win, got %q", yaml.ScalarText(image))
	}
}

func TestExtendYamlService_Direct_MissingBaseServiceReturnsError(t *testing.T) {
	base := mapping(strPair("image", "base-image"))
	local := mapping(strPair("image", "web-image"))
	services := buildServicesNode(pair(str("base"), base), pair(str("web"), local))

	err := ExtendYamlService(services, services, nil, 1, "does-not-exist", nil, nil, true)
	if err == nil {
		t.Fatalf("expected an error when the referenced base service does not exist")
	}
}

// --- JsonToYaml ---

// JsonToYaml decodes JSON via encoding/json into interface{}, which always
// produces float64 for numbers; goccy's Marshal renders that as "1.0"
// (preserving the Go type), so it round-trips back as float64 too.
func TestJsonToYaml_Basic(t *testing.T) {
	got, err := JsonToYaml(`{"a":1,"b":"x","c":[1,2,3],"d":null}`)
	if err != nil {
		t.Fatalf("JsonToYaml failed: %v", err)
	}

	var v interface{}
	if err := yaml.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("failed to parse produced YAML: %v", err)
	}

	want := map[string]interface{}{
		"a": float64(1),
		"b": "x",
		"c": []interface{}{float64(1), float64(2), float64(3)},
		"d": nil,
	}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("got %#v, want %#v", v, want)
	}
}

func TestJsonToYaml_InvalidJSONReturnsError(t *testing.T) {
	_, err := JsonToYaml(`{not valid json`)
	if err == nil {
		t.Fatalf("expected an error for invalid JSON input")
	}
}

// --- LoadYamlFile end-to-end ---

func TestLoadYamlFile_TemplateAndPrepare(t *testing.T) {
	dir := t.TempDir()
	path := writeTempYaml(t, dir, "kraken.yaml", `
services:
  web:
    image: ${IMAGE_NAME}
    scale: 2
`)

	env := func(key string) string {
		switch key {
		case "IMAGE_NAME":
			return "myapp:latest"
		case "APP_ENV":
			return ""
		}
		return ""
	}

	out, err := LoadYamlFile(path, env)
	if err != nil {
		t.Fatalf("LoadYamlFile failed: %v", err)
	}

	var v interface{}
	if err := yaml.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("failed to parse LoadYamlFile output: %v\noutput:\n%s", err, out)
	}

	want := map[string]interface{}{
		"services": map[string]interface{}{
			"web1": map[string]interface{}{"image": "myapp:latest"},
			"web2": map[string]interface{}{"image": "myapp:latest"},
		},
	}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("got %#v, want %#v", v, want)
	}
}

func TestLoadYamlFile_MissingFileReturnsError(t *testing.T) {
	_, err := LoadYamlFile("/nonexistent/kraken.yaml", func(string) string { return "" })
	if err == nil {
		t.Fatalf("expected an error for a missing file")
	}
}
