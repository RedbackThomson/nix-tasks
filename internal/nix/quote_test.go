package nix_test

import (
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/nix"
)

func TestQuoteAttr(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"build", `"build"`},
		{"lint.go", `"lint.go"`},
		{`a\b`, `"a\b"`},
		{"a${b}", `"a${b}"`},
		{"run tests", `"run tests"`},
		{"100%", `"100%25"`},
		{"a%41b", `"a%2541b"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nix.QuoteAttr(tt.name); got != tt.want {
				t.Errorf("QuoteAttr(%q) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestAttrPath(t *testing.T) {
	if got, want := nix.AttrPath("x86_64-linux", "a.b", "50%"), `"x86_64-linux"."a.b"."50%25"`; got != want {
		t.Errorf("AttrPath() = %s, want %s", got, want)
	}
}
