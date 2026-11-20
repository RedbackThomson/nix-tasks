package nix

import "strings"

// QuoteAttr quotes a single attribute name for use in a flake attribute path,
// so names containing "." (e.g. "lint.go") stay one path component. Quotes in
// an attribute path take no escapes, so every other character is written as
// is and a name containing `"` cannot be quoted at all (config validation
// rejects such names). The part of a flake reference after "#" is
// percent-decoded, so "%" is written as "%25".
func QuoteAttr(name string) string {
	return `"` + strings.ReplaceAll(name, "%", "%25") + `"`
}

// AttrPath joins attribute names into a flake attribute path, quoting each.
func AttrPath(names ...string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = QuoteAttr(n)
	}
	return strings.Join(quoted, ".")
}

// NixTasksShellsAttr returns the flake attribute for a task's execution shell.
func NixTasksShellsAttr(system, taskName string) string {
	return "nixTasksShells." + AttrPath(system, taskName)
}

// NixTasksBuildsAttr returns the flake attribute for a build task's derivation.
func NixTasksBuildsAttr(system, taskName string) string {
	return "nixTasksBuilds." + AttrPath(system, taskName)
}

// DevShellAttr returns the flake attribute for a user-facing dev shell.
func DevShellAttr(system, shellName string) string {
	return "devShells." + AttrPath(system, shellName)
}
