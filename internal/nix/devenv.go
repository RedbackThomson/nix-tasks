package nix

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DevEnv is the build environment of a shell derivation, as printed by
// `nix print-dev-env --json`.
type DevEnv struct {
	Variables map[string]DevEnvVar `json:"variables"`
}

// DevEnvVar is one shell variable of a DevEnv. Value is a string for
// "exported" and "var" variables and a list or map for "array" and
// "associative" ones.
type DevEnvVar struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// ParseDevEnv parses `nix print-dev-env --json` output.
func ParseDevEnv(data []byte) (*DevEnv, error) {
	var env DevEnv
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to parse nix print-dev-env output: %w", err)
	}
	if env.Variables == nil {
		return nil, fmt.Errorf("nix print-dev-env output has no variables")
	}
	return &env, nil
}

// devEnvIgnoredVars are left as the host has them, matching nix develop: they
// describe the sandboxed builder that produced the environment, not the
// shell the user runs in.
var devEnvIgnoredVars = map[string]bool{
	"BASHOPTS":           true,
	"HOME":               true,
	"NIX_BUILD_TOP":      true,
	"NIX_ENFORCE_PURITY": true,
	"NIX_LOG_FD":         true,
	"NIX_REMOTE":         true,
	"PPID":               true,
	"SHELLOPTS":          true,
	"SSL_CERT_FILE":      true,
	"TEMP":               true,
	"TEMPDIR":            true,
	"TERM":               true,
	"TMP":                true,
	"TMPDIR":             true,
	"TZ":                 true,
	"UID":                true,
}

// devEnvSearchPaths are prepended to the host's value instead of replacing
// it, as nix develop does, so host tools stay reachable after the shell's.
var devEnvSearchPaths = map[string]bool{
	"PATH":          true,
	"XDG_DATA_DIRS": true,
}

func (e *DevEnv) str(name string) (string, bool) {
	v, ok := e.Variables[name]
	if !ok || (v.Type != "exported" && v.Type != "var") {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err != nil {
		return "", false
	}
	return s, true
}

// Environ returns host with the shell's exported variables applied, for a
// process running in dir. Like nix develop, it points the derivation's
// outputs ($out, ...) at dir/outputs/<name> instead of unbuilt store paths.
// Non-exported variables and bash functions are stdenv internals that a
// task's script and the processes it starts don't see under nix develop
// --command either, so they are dropped.
func (e *DevEnv) Environ(host []string, dir string) []string {
	vars := make(map[string]string, len(host)+len(e.Variables))
	for _, kv := range host {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}

	var rewrites []string
	if outputs, ok := e.str("outputs"); ok {
		for _, name := range strings.Fields(outputs) {
			if path, ok := e.str(name); ok && path != "" {
				rewrites = append(rewrites, path, filepath.Join(dir, "outputs", name))
			}
		}
	}
	rewrite := strings.NewReplacer(rewrites...)

	for name, v := range e.Variables {
		if v.Type != "exported" || devEnvIgnoredVars[name] {
			continue
		}
		value, ok := e.str(name)
		if !ok {
			continue
		}
		value = rewrite.Replace(value)
		if devEnvSearchPaths[name] && vars[name] != "" {
			if value == "" {
				value = vars[name]
			} else {
				value += ":" + vars[name]
			}
		}
		vars[name] = value
	}

	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
