package cli_test

import (
	"testing"

	"github.com/alecthomas/kong"

	"github.com/redbackthomson/nix-tasks/internal/cli"
)

func TestStreamIsVerboseAlias(t *testing.T) {
	for _, args := range [][]string{
		{"run", "build", "-v"},
		{"run", "build", "--verbose"},
		{"run", "build", "--stream"},
		{"--stream", "run", "build"},
	} {
		var c cli.CLI
		parser, err := kong.New(&c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !c.Verbose {
			t.Errorf("%v: Verbose not set", args)
		}
	}
}

func TestColorFlag(t *testing.T) {
	var c cli.CLI
	parser, err := kong.New(&c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"--color=never", "list"}); err != nil {
		t.Fatal(err)
	}
	if c.Color != "never" {
		t.Errorf("Color = %q", c.Color)
	}
	if _, err := parser.Parse([]string{"--color=sometimes", "list"}); err == nil {
		t.Error("expected an invalid --color value to be rejected")
	}
}

func TestJobsMustBePositive(t *testing.T) {
	for _, tt := range []struct {
		jobs    string
		wantErr bool
	}{
		{"1", false},
		{"8", false},
		{"0", true},
		{"-2", true},
	} {
		var c cli.CLI
		parser, err := kong.New(&c)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parser.Parse([]string{"run", "build", "-j", tt.jobs})
		if (err != nil) != tt.wantErr {
			t.Errorf("-j %s: error = %v, wantErr %v", tt.jobs, err, tt.wantErr)
		}
	}
}

func TestOutputFlag(t *testing.T) {
	var c cli.CLI
	parser, err := kong.New(&c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"run", "build", "--output=plain"}); err != nil {
		t.Fatal(err)
	}
	if c.Output != "plain" {
		t.Errorf("Output = %q", c.Output)
	}
	if _, err := parser.Parse([]string{"--output=fancy", "run", "build"}); err == nil {
		t.Error("expected an invalid --output value to be rejected")
	}
}
