package runner

import (
	"bytes"
	"sync"
	"testing"
)

func TestPrefixWriter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "single line", input: "hello\n", want: "[t] hello\n"},
		{name: "multiple lines", input: "a\nb\n", want: "[t] a\n[t] b\n"},
		{name: "unterminated line", input: "a", want: "[t] a\n"},
		{name: "blank line kept", input: "a\n\nb\n", want: "[t] a\n[t] \n[t] b\n"},
		{name: "carriage returns stripped", input: "10%\r50%\r100%\n", want: "[t] 10%50%100%\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := &prefixWriter{prefix: "[t] ", out: &out, mu: &sync.Mutex{}}
			n, err := w.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if n != len(tt.input) {
				t.Errorf("Write() n = %d, want %d", n, len(tt.input))
			}
			if got := out.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}
