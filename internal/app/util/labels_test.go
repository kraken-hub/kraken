package util

import "testing"

func TestAppNetwork(t *testing.T) {
	if got := AppNetwork("myapp"); got != "kraken_net.myapp" {
		t.Errorf("AppNetwork() = %q, want kraken_net.myapp", got)
	}
}

func TestContainerName(t *testing.T) {
	if got := ContainerName("myapp", "web"); got != "web.myapp" {
		t.Errorf("ContainerName() = %q, want web.myapp", got)
	}
}

func TestAppLabels(t *testing.T) {
	got := AppLabels("myapp")
	want := map[string]string{"kraken.app": "myapp"}
	if len(got) != len(want) || got["kraken.app"] != "myapp" {
		t.Errorf("AppLabels() = %#v, want %#v", got, want)
	}
}

func TestBuiltImageName(t *testing.T) {
	if got := BuiltImageName("myapp", "web"); got != "kraken_myapp_web" {
		t.Errorf("BuiltImageName() = %q, want kraken_myapp_web", got)
	}
}

func TestLabelConstants(t *testing.T) {
	if LabelApp != "kraken.app" {
		t.Errorf("LabelApp = %q", LabelApp)
	}
	if LabelService != "kraken.service" {
		t.Errorf("LabelService = %q", LabelService)
	}
	if GlobalNetwork != "kraken_net" {
		t.Errorf("GlobalNetwork = %q", GlobalNetwork)
	}
}
