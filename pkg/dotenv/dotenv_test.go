package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadBasic(t *testing.T) {
	path := writeEnvFile(t, "FOO=bar\nBAZ=qux\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadCommentsAndBlankLines(t *testing.T) {
	path := writeEnvFile(t, "# a comment\n\n   \nFOO=bar\n# another\nBAZ=qux\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got["FOO"] != "bar" || got["BAZ"] != "qux" {
		t.Errorf("got %v", got)
	}
}

func TestLoadExportKeyword(t *testing.T) {
	path := writeEnvFile(t, "export FOO=bar\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("got %v", got)
	}
}

func TestLoadVariableExpansionWithinFile(t *testing.T) {
	path := writeEnvFile(t, "BASE_URL=http://example.com\nAPI=${BASE_URL}/api\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["BASE_URL"] != "http://example.com" {
		t.Errorf("BASE_URL = %q", got["BASE_URL"])
	}
	if got["API"] != "http://example.com/api" {
		t.Errorf("API = %q, want expansion of BASE_URL", got["API"])
	}
}

func TestLoadQuotedValueWithSpaces(t *testing.T) {
	path := writeEnvFile(t, `QUOTED="hello world"`+"\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["QUOTED"] != "hello world" {
		t.Errorf("got %q", got["QUOTED"])
	}
}

// TestLoadDoesNotLeakHostEnv locks in the fix for a real bug: interp.New() used
// to inherit the entire host process environment, so an unrelated host env var
// (never assigned in the .env file) would incorrectly show up in the result.
func TestLoadDoesNotLeakHostEnv(t *testing.T) {
	t.Setenv("KRAKEN_TEST_HOST_SECRET", "leaked-value")

	path := writeEnvFile(t, "FOO=bar\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got["KRAKEN_TEST_HOST_SECRET"]; ok {
		t.Errorf("host env var leaked into parsed result: %v", got)
	}
	if len(got) != 1 || got["FOO"] != "bar" {
		t.Errorf("got %v, want only FOO=bar", got)
	}
}

// TestLoadDoesNotLeakShellBuiltins ensures interpreter built-in vars (IFS, PWD,
// HOME, OPTIND, etc.) that the shell runner sets itself are excluded, since they
// were never explicitly assigned in the file.
func TestLoadDoesNotLeakShellBuiltins(t *testing.T) {
	path := writeEnvFile(t, "FOO=bar\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, builtin := range []string{"IFS", "PWD", "HOME", "OPTIND", "PATH"} {
		if _, ok := got[builtin]; ok {
			t.Errorf("unexpected builtin %q present in result: %v", builtin, got)
		}
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := writeEnvFile(t, "")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadInvalidSyntax(t *testing.T) {
	path := writeEnvFile(t, "this is not = valid shell (((")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid syntax, got nil")
	}
}
