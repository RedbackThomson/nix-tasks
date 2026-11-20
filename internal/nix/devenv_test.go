package nix_test

import (
	"slices"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

const sampleDevEnv = `{
  "bashFunctions": {"genericBuild": "..."},
  "variables": {
    "PATH": {"type": "exported", "value": "/nix/store/go/bin"},
    "XDG_DATA_DIRS": {"type": "exported", "value": ""},
    "GOTOOLCHAIN": {"type": "exported", "value": "local"},
    "HOME": {"type": "exported", "value": "/homeless-shelter"},
    "TMPDIR": {"type": "exported", "value": "/nix/var/nix/builds/x"},
    "out": {"type": "exported", "value": "/nix/store/abc-nix-shell-env"},
    "outputs": {"type": "exported", "value": "out"},
    "OUT_BIN": {"type": "exported", "value": "/nix/store/abc-nix-shell-env/bin"},
    "prefix": {"type": "var", "value": "/nix/store/abc-nix-shell-env"},
    "pkgsHostTarget": {"type": "array", "value": ["/nix/store/go"]}
  }
}`

func TestDevEnv_Environ(t *testing.T) {
	env, err := nix.ParseDevEnv([]byte(sampleDevEnv))
	if err != nil {
		t.Fatal(err)
	}
	host := []string{
		"PATH=/usr/bin:/bin",
		"XDG_DATA_DIRS=/usr/share",
		"HOME=/Users/me",
		"TMPDIR=/tmp/me",
		"EDITOR=vi",
	}
	got := env.Environ(host, "/repo")
	want := []string{
		"EDITOR=vi",
		"GOTOOLCHAIN=local",
		"HOME=/Users/me",
		"OUT_BIN=/repo/outputs/out/bin",
		"PATH=/nix/store/go/bin:/usr/bin:/bin",
		"TMPDIR=/tmp/me",
		"XDG_DATA_DIRS=/usr/share",
		"out=/repo/outputs/out",
		"outputs=out",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Environ() =\n  %v\nwant\n  %v", got, want)
	}
}

func TestDevEnv_EnvironWithoutHostPath(t *testing.T) {
	env, err := nix.ParseDevEnv([]byte(sampleDevEnv))
	if err != nil {
		t.Fatal(err)
	}
	got := env.Environ(nil, "/repo")
	if !slices.Contains(got, "PATH=/nix/store/go/bin") {
		t.Errorf("PATH should be the shell's alone, got %v", got)
	}
}

func TestParseDevEnv_Invalid(t *testing.T) {
	for _, input := range []string{"", "not json", `{"bashFunctions": {}}`} {
		if _, err := nix.ParseDevEnv([]byte(input)); err == nil {
			t.Errorf("ParseDevEnv(%q) should fail", input)
		}
	}
}

func TestNixString(t *testing.T) {
	tests := map[string]string{
		`plain`:          `"plain"`,
		`{"a":null}`:     `"{\"a\":null}"`,
		`back\slash`:     `"back\\slash"`,
		`${interpolate}`: `"\${interpolate}"`,
		"new\nline":      `"new\nline"`,
		`$notInterp`:     `"$notInterp"`,
	}
	for in, want := range tests {
		if got := nix.NixString(in); got != want {
			t.Errorf("NixString(%q) = %s, want %s", in, got, want)
		}
	}
}
