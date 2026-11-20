package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"
	"github.com/posener/complete"
)

// With fakeCLIEnv set, the test binary stands in for nix-tasks: it answers
// completion requests through Complete, and answers `list --names` with fixed
// names plus "in-<dir>" naming the -f flake, so tests can see -f arrive.
const fakeCLIEnv = "NIX_TASKS_FAKE_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(fakeCLIEnv) != "" {
		fakeCLI(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeCLI(args []string) {
	var c CLI
	Complete(kong.Must(&c, kong.Name("nix-tasks")))

	names := []string{"build", "lint", "test"}
	if slices.Contains(args, "--shells") {
		names = []string{"default", "extra"}
	}
	if i := slices.Index(args, "-f"); i >= 0 && i+1 < len(args) {
		names = append(names, "in-"+filepath.Base(args[i+1]))
	}
	fmt.Println(strings.Join(names, "\n"))
	os.Exit(0)
}

// completionCommand returns the completion model Complete builds from the CLI,
// with dynamic names stubbed out.
func completionCommand(t *testing.T) (complete.Command, *kong.Kong) {
	t.Helper()
	var c CLI
	parser := kong.Must(&c, kong.Name("nix-tasks"))
	nothing := complete.PredictNothing
	cmd, err := kongcompletion.Command(parser,
		kongcompletion.WithPredictor("task", nothing),
		kongcompletion.WithPredictor("shell", nothing),
		kongcompletion.WithPredictor("dir", nothing),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cmd, parser
}

func TestCompletions_CoverEveryCommandAndFlag(t *testing.T) {
	root, parser := completionCommand(t)

	var walk func(n *kong.Node, cmd complete.Command)
	walk = func(n *kong.Node, cmd complete.Command) {
		for _, f := range n.Flags {
			if _, ok := cmd.GlobalFlags["--"+f.Name]; ok == f.Hidden {
				t.Errorf("%s: flag --%s offered = %v, hidden = %v", n.Path(), f.Name, ok, f.Hidden)
			}
		}
		for _, child := range n.Children {
			sub, ok := cmd.Sub[child.Name]
			if ok == child.Hidden {
				t.Errorf("%s: command %s offered = %v, hidden = %v", n.Path(), child.Name, ok, child.Hidden)
			}
			if ok {
				walk(child, sub)
			}
		}
	}
	walk(parser.Model.Node, root)
}

func TestFlakeArg(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		line   string
		want   string
		wantOK bool
	}{
		{"nix-tasks run ", "", false},
		{"nix-tasks -f ../x run ", "../x", true},
		{"nix-tasks --flake ../x run ", "../x", true},
		{"nix-tasks --flake=../x run ", "../x", true},
		{"nix-tasks -f a run -f b ", "b", true},
		{"nix-tasks -f ~/x run ", home + "/x", true},
		{"nix-tasks run -f", "", false},
	}
	for _, tt := range tests {
		got, ok := flakeArg(strings.Fields(tt.line))
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("flakeArg(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.wantOK)
		}
	}
}

// shellHarness completes a command line in a real shell, with the test binary
// standing in for nix-tasks, and prints the candidates.
type shellHarness struct {
	argv func(script, line string) []string
}

var shellHarnesses = map[string]shellHarness{
	"bash": {argv: func(script, line string) []string {
		return []string{"bash", "-c", `
type compgen >/dev/null 2>&1 || { echo SKIP; exit 0; }
source "$1"
nix-tasks() { "$NIX_TASKS_TEST_BIN" "$@"; }
COMP_LINE="$2" COMP_POINT=${#2}
_nix_tasks
echo "${COMPREPLY[*]}"`, "bash", script, line}
	}},
	"zsh": {argv: func(script, line string) []string {
		return []string{"zsh", "-f", "-c", `
compdef() { :; }
compadd() {
    while [[ $1 == -* ]]; do
        case $1 in
            --) shift; break ;;
            -S) shift 2 ;;
            *) shift ;;
        esac
    done
    echo "$@"
}
nix-tasks() { "$NIX_TASKS_TEST_BIN" "$@"; }
source "$1"
words=(${=2})
[[ $2 == *' ' ]] && words+=("")
CURRENT=${#words}
_nix_tasks; true`, "zsh", script, line}
	}},
	"fish": {argv: func(script, line string) []string {
		return []string{"fish", "-c", `
source $argv[1]
function nix-tasks; $NIX_TASKS_TEST_BIN $argv; end
complete -C "$argv[2]" | string replace -r '\t.*' '' | string join ' '; true`, script, line}
	}},
}

func TestCompletions_InRealShells(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	scripts := t.TempDir()
	for shell, script := range completionScripts {
		if err := os.WriteFile(filepath.Join(scripts, shell), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	for _, d := range []string{"flakes/one", "flakes/two"} {
		if err := os.MkdirAll(filepath.Join(work, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		line string
		want string
	}{
		{"nix-tasks gr", "graph"},
		{"nix-tasks run ", "build lint test"},
		{"nix-tasks run build ", ""},
		{"nix-tasks run --contin", "--continue-on-error"},
		{"nix-tasks -f flakes/one run ", "build in-one lint test"},
		{"nix-tasks describe -f flakes/two ", "build in-two lint test"},
		{"nix-tasks describe build ", ""},
		{"nix-tasks shell ", "default extra"},
		{"nix-tasks --color ", "always auto never"},
		{"nix-tasks -f flakes/", "flakes/ flakes/one/ flakes/two/"},
		{"nix-tasks list --na", ""},
	}
	for shell, h := range shellHarnesses {
		if _, err := exec.LookPath(shell); err != nil {
			t.Logf("%s not available", shell)
			continue
		}
		for _, tt := range tests {
			t.Run(shell+"/"+tt.line, func(t *testing.T) {
				argv := h.argv(filepath.Join(scripts, shell), tt.line)
				cmd := exec.Command(argv[0], argv[1:]...)
				cmd.Dir = work
				cmd.Env = append(os.Environ(), fakeCLIEnv+"=1", "NIX_TASKS_TEST_BIN="+bin)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v\n%s", shell, err, out)
				}
				if strings.TrimSpace(string(out)) == "SKIP" {
					t.Skip("bash built without programmable completion")
				}
				got := strings.Fields(string(out))
				slices.Sort(got)
				if strings.Join(got, " ") != tt.want {
					t.Errorf("completions = %q, want %q", strings.Join(got, " "), tt.want)
				}
			})
		}
	}
}

// installShellCompletion installs the zsh script as _nix-tasks, so the
// script must complete on the first TAB when autoloaded under that name and
// register itself with compdef when sourced.
func TestCompletions_ZshAutoloadAndSource(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_nix-tasks"), []byte(completionScripts["zsh"]), 0o644); err != nil {
		t.Fatal(err)
	}

	script := `fpath=("$1" $fpath)
compdef() { echo "compdef $*"; }
compadd() { echo "compadd ${@:#--}"; }
nix-tasks() { "$NIX_TASKS_TEST_BIN" "$@"; }
autoload -Uz _nix-tasks
words=(nix-tasks run ""); CURRENT=3
_nix-tasks
source "$1/_nix-tasks"`
	cmd := exec.Command("zsh", "-f", "-c", script, "zsh", dir)
	cmd.Env = append(os.Environ(), fakeCLIEnv+"=1", "NIX_TASKS_TEST_BIN="+bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh failed: %v\n%s", err, out)
	}
	if want := "compadd build lint test\ncompdef _nix_tasks nix-tasks\n"; string(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
