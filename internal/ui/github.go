package ui

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// DefaultPalette colours task prefixes: cyan, magenta, blue and their bright
// variants. The status colours are left out so a prefix never reads as a
// result.
var DefaultPalette = []Color{6, 5, 4, 14, 13, 12}

// GitHubOptions configures a GitHubReporter.
type GitHubOptions struct {
	Out io.Writer
	// SummaryPath is the job summary file, $GITHUB_STEP_SUMMARY; empty
	// writes no summary.
	SummaryPath string
	Now         func() time.Time
	// Palette defaults to DefaultPalette.
	Palette []Color
	// Token returns the stop-commands token; it defaults to a random one,
	// so task output cannot guess it and resume command processing.
	Token func() string
	// SpoolDir holds each task's output until the run ends; it defaults to
	// a new temp directory, removed by Stop.
	SpoolDir string
}

// GitHubReporter writes a run for the GitHub Actions log viewer. Output
// streams live with coloured, aligned task prefixes, since ::group:: folding
// cannot hold interleaved output; the folded per-task view comes after the
// run, in a Results section, the job summary and annotations.
type GitHubReporter struct {
	opts  GitHubOptions
	start time.Time

	mu             sync.Mutex // serializes whole-line writes to Out
	plan           runner.RunPlan
	colors         map[string]Color
	width          int
	prefixes       bool
	configDuration time.Duration
	tasks          map[string]*githubTask
	masks          []string
	buf            []byte

	// stopToken is set while a task's ::stop-commands:: block is open, when
	// GitHub ignores every command until ::<stopToken>::.
	stopToken        string
	stopOwner        string
	pendingEndgroups int
	spoolDir         string
	ownSpoolDir      bool
	stopOnce         sync.Once
}

type githubTask struct {
	g    *GitHubReporter
	name string

	mu    sync.Mutex
	spool *os.File
	path  string
	lines lineSplitter

	// prefix starts each line of output that is not a workflow command,
	// unless alwaysPrefix is unset (a run of one task).
	prefix       string
	alwaysPrefix bool

	// groups counts the ::group:: commands the task has not closed; guarded
	// by g.mu.
	groups int
}

func (t *githubTask) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.spool != nil {
		_, _ = t.spool.Write(p)
	}
	t.lines.feed(p, t.emit)
	return len(p), nil
}

func (t *githubTask) emit(line []byte) {
	t.g.mu.Lock()
	defer t.g.mu.Unlock()
	t.g.taskLineLocked(t, line)
}

// parseWorkflowCommand parses line the way the GitHub runner does: after any
// leading whitespace, "::" then the command up to the next "::", where the
// command is a name optionally followed by a space and parameters.
func parseWorkflowCommand(line []byte) (name string, value string, ok bool) {
	rest, found := bytes.CutPrefix(bytes.TrimLeftFunc(line, unicode.IsSpace), []byte("::"))
	if !found {
		return "", "", false
	}
	end := bytes.Index(rest, []byte("::"))
	if end < 0 {
		return "", "", false
	}
	name, _, _ = strings.Cut(string(rest[:end]), " ")
	if name == "" {
		return "", "", false
	}
	return name, string(rest[end+2:]), true
}

// taskLineLocked writes one line of a task's output. Workflow commands are
// passed through unprefixed so tools behave as they would without
// nix-tasks; the reporter tracks the ones that could break its own layout.
// GitHub ignores every command while a stop-commands block is open.
func (g *GitHubReporter) taskLineLocked(t *githubTask, line []byte) {
	name, value, isCommand := parseWorkflowCommand(line)
	prefixed := t.alwaysPrefix && !isCommand
	switch {
	case !isCommand:
	case g.stopToken != "":
		if name == g.stopToken {
			g.resumeLocked()
		}
	case name == "stop-commands" && strings.TrimSpace(value) != "":
		g.stopToken, g.stopOwner = strings.TrimSpace(value), t.name
	case name == "group":
		t.groups++
	case name == "endgroup":
		// Only a group the task opened may be closed, never nix-tasks' own.
		if t.groups == 0 {
			prefixed = true
		} else {
			t.groups--
		}
	case name == "add-mask":
		if v := strings.TrimSpace(value); v != "" {
			g.masks = append(g.masks, v)
		}
	}
	g.buf = g.buf[:0]
	if prefixed {
		g.buf = append(g.buf, t.prefix...)
	}
	g.buf = append(g.buf, line...)
	g.buf = append(g.buf, '\n')
	_, _ = g.opts.Out.Write(g.buf)
}

// resumeLocked ends a tool's stop-commands block, then closes any groups
// whose ::endgroup:: had to wait for it.
func (g *GitHubReporter) resumeLocked() {
	g.stopToken, g.stopOwner = "", ""
	for ; g.pendingEndgroups > 0; g.pendingEndgroups-- {
		g.writeLocked("::endgroup::")
	}
}

// finishTaskLocked undoes what a finished task left behind: a stop-commands
// block it never resumed and groups it never closed.
func (g *GitHubReporter) finishTaskLocked(t *githubTask) {
	if g.stopToken != "" && g.stopOwner == t.name {
		g.writeLocked("::" + g.stopToken + "::")
		g.resumeLocked()
	}
	if g.stopToken != "" {
		g.pendingEndgroups += t.groups
	} else {
		for range t.groups {
			g.writeLocked("::endgroup::")
		}
	}
	t.groups = 0
}

func (t *githubTask) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines.flush(t.emit)
	t.g.mu.Lock()
	t.g.finishTaskLocked(t)
	t.g.mu.Unlock()
	if t.spool != nil {
		_ = t.spool.Close()
		t.spool = nil
	}
}

// NewGitHubReporter creates a GitHubReporter.
func NewGitHubReporter(opts GitHubOptions) *GitHubReporter {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if len(opts.Palette) == 0 {
		opts.Palette = DefaultPalette
	}
	if opts.Token == nil {
		opts.Token = randomToken
	}
	return &GitHubReporter{
		opts:     opts,
		start:    opts.Now(),
		colors:   map[string]Color{},
		tasks:    map[string]*githubTask{},
		spoolDir: opts.SpoolDir,
	}
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (g *GitHubReporter) Start()     {}
func (g *GitHubReporter) Interrupt() {}

// Stop removes the spooled task output.
func (g *GitHubReporter) Stop() {
	g.stopOnce.Do(func() {
		g.mu.Lock()
		tasks := slices.Collect(maps.Values(g.tasks))
		dir, own := g.spoolDir, g.ownSpoolDir
		g.mu.Unlock()
		// Closing flushes partial lines, which takes g.mu.
		for _, t := range tasks {
			t.close()
		}
		if own {
			_ = os.RemoveAll(dir)
		}
	})
}

// ConfigLoaded records how long evaluating the configuration took, for the
// setup group.
func (g *GitHubReporter) ConfigLoaded(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.configDuration = d
}

func (g *GitHubReporter) RunPlanned(plan runner.RunPlan) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.plan = plan
	g.prefixes = len(plan.Tasks) > 1
	for i, name := range slices.Sorted(slices.Values(plan.Tasks)) {
		g.colors[name] = g.opts.Palette[i%len(g.opts.Palette)]
		g.width = max(g.width, ansi.StringWidth(g.label(name)))
	}
	g.writeLocked(fmt.Sprintf("nix-tasks: %s · %s · -j %d", strings.Join(plan.Targets, " "), plural(len(plan.Tasks), "task", "tasks"), plan.Jobs))
}

// SetupFinished prints environment preparation as one collapsed group. It
// runs before any task starts, so nothing else can land inside the group.
func (g *GitHubReporter) SetupFinished(envs []runner.EnvSetup, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	total := d + g.configDuration
	title := "Loading configuration " + formatSeconds(total, true)
	if len(envs) > 0 {
		title = "Preparing " + plural(len(envs), "environment", "environments") + " " + formatSeconds(total, true)
	}
	lines := []string{"::group::" + title}
	if g.configDuration > 0 {
		lines = append(lines, "evaluated nixTasksConfig "+formatSeconds(g.configDuration, true))
	}
	var failures []string
	for _, env := range envs {
		what := "environment for " + strings.Join(env.Tasks, ", ") + ": "
		switch {
		case env.Err != nil:
			what += "failed"
			failures = append(failures, env.Err.Error())
		case env.Cached:
			what += "cached environment"
		default:
			what += "built with nix print-dev-env"
		}
		lines = append(lines, what+" "+formatSeconds(env.Duration, true))
	}
	if len(failures) > 0 {
		token := g.opts.Token()
		lines = append(lines, "::stop-commands::"+token)
		for _, f := range failures {
			lines = append(lines, splitLines([]byte(f))...)
		}
		lines = append(lines, "::"+token+"::")
	}
	lines = append(lines, "::endgroup::")
	g.writeLocked(lines...)
}

func (g *GitHubReporter) TaskStarted(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	shown := name
	if g.prefixes {
		shown = withLabel(name, g.label(name))
	}
	g.writeLocked(sgr("2;"+g.color(name).sgr(), "▶ "+shown))
}

func (g *GitHubReporter) TaskOutput(name string) (io.Writer, io.Writer) {
	g.mu.Lock()
	defer g.mu.Unlock()
	t, ok := g.tasks[name]
	if !ok {
		label := g.label(name)
		pad := ""
		if g.prefixes {
			pad = strings.Repeat(" ", max(g.width-ansi.StringWidth(label), 0))
		}
		t = &githubTask{
			g:            g,
			name:         name,
			prefix:       g.color(name).paint(label+pad+" │") + " ",
			alwaysPrefix: g.prefixes,
		}
		if f, err := g.createSpool(name); err == nil {
			t.spool, t.path = f, f.Name()
		}
		g.tasks[name] = t
	}
	return t, t
}

func (g *GitHubReporter) createSpool(name string) (*os.File, error) {
	if g.spoolDir == "" {
		dir, err := os.MkdirTemp("", "nix-tasks-github-")
		if err != nil {
			return nil, err
		}
		g.spoolDir, g.ownSpoolDir = dir, true
	}
	return os.CreateTemp(g.spoolDir, strings.ReplaceAll(name, "/", "_")+"-*.log")
}

func (g *GitHubReporter) TaskFinished(r runner.TaskResult) {
	g.mu.Lock()
	t := g.tasks[r.Name]
	g.mu.Unlock()
	if t != nil {
		t.close()
	}

	var style, text string
	switch {
	case r.State == runner.Succeeded && r.Cached:
		style, text = "1;"+AccentColor.sgr(), "● "+r.Name+" cached"
	case r.State == runner.Succeeded:
		style, text = "1;"+SuccessColor.sgr(), "✓ "+r.Name+" · "+formatSeconds(r.Duration, false)
	case r.State == runner.Failed:
		style, text = "1;"+FailureColor.sgr(), "✗ "+r.Name+" failed"+exitSuffix(r)+" · "+formatSeconds(r.Duration, false)
	case r.State == runner.FailedAllowed:
		style, text = "1;"+WarningColor.sgr(), "! "+r.Name+" failed, allowed"+exitSuffix(r)+" · "+formatSeconds(r.Duration, false)
	case r.State == runner.Skipped:
		style, text = MutedColor.sgr(), "- "+r.Name+" skipped"
		if r.SkipReason != "" {
			text += " · " + r.SkipReason
		}
	default:
		style, text = MutedColor.sgr(), "⏹ "+r.Name+" interrupted · "+formatSeconds(r.Duration, false)
	}
	g.writeLine(sgr(style, "━━ "+text+" ━━"))
}

// Summary prints the Results section and annotations, and writes the job
// summary. It lists tasks in plan order rather than as a tree, so it does not
// need the graph or targets.
func (g *GitHubReporter) Summary(_ *runner.TaskGraph, _ []string, results []runner.TaskResult) {
	g.mu.Lock()
	defer g.mu.Unlock()
	results = g.inPlanOrder(results)
	elapsed := g.opts.Now().Sub(g.start)
	if g.stopToken != "" {
		g.writeLocked("::" + g.stopToken + "::")
		g.resumeLocked()
	}

	var b strings.Builder
	rule := strings.Repeat("━", 26)
	b.WriteString("\n" + rule + " Results " + rule + "\n")
	for _, r := range results {
		if !failed(r) {
			b.WriteString(resultLine(r) + "\n")
			continue
		}
		g.writeResultGroup(&b, r)
	}
	b.WriteString("\n" + totalsLine(results, elapsed) + "\n")
	for _, a := range annotations(results) {
		b.WriteString(a + "\n")
	}
	_, _ = io.WriteString(g.opts.Out, b.String())

	if g.opts.SummaryPath != "" {
		if err := g.writeJobSummary(results, elapsed); err != nil {
			_, _ = fmt.Fprintf(g.opts.Out, "nix-tasks: failed to write the job summary: %v\n", err)
		}
	}
}

// writeResultGroup writes a failed task's whole output in a group. The
// output has no prefix here, so stop-commands keeps a line such as
// "::endgroup::" from being read as a workflow command.
func (g *GitHubReporter) writeResultGroup(b *strings.Builder, r runner.TaskResult) {
	b.WriteString("::group::" + ansi.Strip(resultLine(r)) + "\n")
	token := g.opts.Token()
	b.WriteString("::stop-commands::" + token + "\n")
	if t := g.tasks[r.Name]; t != nil && t.path != "" {
		_ = eachLine(t.path, func(line string) {
			if !ColorEnabled() {
				line = ansi.Strip(line)
			}
			b.WriteString(g.redact(line) + "\n")
		})
	}
	if detail := errorDetail(r); detail != "" {
		b.WriteString(Failure("error: "+detail) + "\n")
	}
	b.WriteString("::" + token + "::\n")
	b.WriteString("::endgroup::\n")
}

// redact hides the values tools asked GitHub to mask. GitHub masks them in
// the log once it has seen the command, which the live output printed
// first, but the job summary is ours to protect.
func (g *GitHubReporter) redact(line string) string {
	for _, m := range g.masks {
		line = strings.ReplaceAll(line, m, "***")
	}
	return line
}

// inPlanOrder sorts results into the plan's dependency order.
func (g *GitHubReporter) inPlanOrder(results []runner.TaskResult) []runner.TaskResult {
	pos := make(map[string]int, len(g.plan.Tasks))
	for i, name := range g.plan.Tasks {
		pos[name] = i
	}
	sorted := slices.Clone(results)
	slices.SortStableFunc(sorted, func(a, b runner.TaskResult) int {
		pa, oka := pos[a.Name]
		pb, okb := pos[b.Name]
		switch {
		case oka && okb:
			return cmp.Compare(pa, pb)
		case oka:
			return -1
		case okb:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}

func (g *GitHubReporter) label(name string) string {
	return labelOf(g.plan.Labels, name)
}

func (g *GitHubReporter) color(name string) Color {
	if c, ok := g.colors[name]; ok {
		return c
	}
	return g.opts.Palette[0]
}

func (g *GitHubReporter) writeLine(line string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.writeLocked(line)
}

func (g *GitHubReporter) writeLocked(lines ...string) {
	_, _ = io.WriteString(g.opts.Out, strings.Join(lines, "\n")+"\n")
}

// resultLine is a task's status in the Results section.
func resultLine(r runner.TaskResult) string {
	switch {
	case r.State == runner.Succeeded && r.Cached:
		return Accent("●") + " " + r.Name + " · cached"
	case r.State == runner.Succeeded:
		return Success("✓") + " " + r.Name + " · " + formatSeconds(r.Duration, false)
	case r.State == runner.Failed:
		return Failure("✗") + " " + r.Name + exitSuffix(r) + " · " + formatSeconds(r.Duration, false)
	case r.State == runner.FailedAllowed:
		return Warning("!") + " " + r.Name + " · failed, allowed" + exitSuffix(r) + " · " + formatSeconds(r.Duration, false)
	case r.State == runner.Skipped:
		line := Muted("-") + " " + r.Name + " · skipped"
		if r.SkipReason != "" {
			line += " (" + r.SkipReason + ")"
		}
		return line
	}
	return Muted("⏹") + " " + r.Name + " · interrupted · " + formatSeconds(r.Duration, false)
}

type runCounts struct {
	failed, allowed, passed, cached, skipped, interrupted int
}

func countResults(results []runner.TaskResult) runCounts {
	var c runCounts
	for _, r := range results {
		switch {
		case r.State == runner.Succeeded && r.Cached:
			c.cached++
		case r.State == runner.Succeeded:
			c.passed++
		case r.State == runner.Failed:
			c.failed++
		case r.State == runner.FailedAllowed:
			c.allowed++
		case r.State == runner.Skipped:
			c.skipped++
		default:
			c.interrupted++
		}
	}
	return c
}

// parts lists the non-zero counts, each passed through style with its kind.
func (c runCounts) parts(style func(kind, s string) string) []string {
	var parts []string
	add := func(kind string, n int, one, many string) {
		if n > 0 {
			parts = append(parts, style(kind, plural(n, one, many)))
		}
	}
	add("failed", c.failed, "failed", "failed")
	add("allowed", c.allowed, "allowed failure", "allowed failures")
	add("passed", c.passed, "passed", "passed")
	add("cached", c.cached, "cached", "cached")
	add("skipped", c.skipped, "skipped", "skipped")
	add("interrupted", c.interrupted, "interrupted", "interrupted")
	return parts
}

func (c runCounts) marker() string {
	switch {
	case c.failed > 0:
		return "✗"
	case c.interrupted > 0:
		return "⏹"
	}
	return "✓"
}

func totalsLine(results []runner.TaskResult, elapsed time.Duration) string {
	c := countResults(results)
	parts := c.parts(func(kind, s string) string {
		switch kind {
		case "failed":
			return Failure(s)
		case "allowed":
			return Warning(s)
		}
		return s
	})
	marker := c.marker()
	switch marker {
	case "✗":
		marker = Failure(marker)
	case "⏹":
		marker = Muted(marker)
	default:
		marker = Success(marker)
	}
	return marker + " " + strings.Join(append(parts, formatSeconds(elapsed, false)), " · ")
}

// maxAnnotations is how many annotations of each level GitHub shows per step.
const maxAnnotations = 10

// annotations returns an ::error for each failed task and a ::warning for
// each allowed failure. Past GitHub's limit, the last one of each level
// counts the rest instead.
func annotations(results []runner.TaskResult) []string {
	var errs, warns []string
	for _, r := range results {
		switch r.State {
		case runner.Failed:
			msg := failureMessage(r) + `. Expand "✗ ` + r.Name + `" in the Results section of the log.`
			errs = append(errs, workflowCommand("error", "nix-tasks: "+r.Name+" failed", msg))
		case runner.FailedAllowed:
			msg := failureMessage(r) + "; continueOnError is set, so the run continued."
			warns = append(warns, workflowCommand("warning", "nix-tasks: "+r.Name+" failed (allowed)", msg))
		}
	}
	if n := len(errs); n > maxAnnotations {
		errs = append(errs[:maxAnnotations-1], workflowCommand("error", "nix-tasks: more failures",
			fmt.Sprintf("…and %d more failed tasks; see the job summary", n-maxAnnotations+1)))
	}
	if n := len(warns); n > maxAnnotations {
		warns = append(warns[:maxAnnotations-1], workflowCommand("warning", "nix-tasks: more allowed failures",
			fmt.Sprintf("…and %d more allowed failures; see the job summary", n-maxAnnotations+1)))
	}
	return append(errs, warns...)
}

// failureMessage says how a task failed, e.g. "lint exited with code 1
// after 3.1s".
func failureMessage(r runner.TaskResult) string {
	after := " after " + formatSeconds(r.Duration, false)
	if code := r.ExitCode(); code > 0 {
		return fmt.Sprintf("%s exited with code %d%s", r.Name, code, after)
	}
	if detail := errorDetail(r); detail != "" {
		return r.Name + " failed" + after + ": " + detail
	}
	return r.Name + " failed" + after
}

// workflowCommand formats a GitHub workflow command with a title, escaped
// so that the message and title cannot end the command early.
func workflowCommand(command, title, message string) string {
	data := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	property := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return "::" + command + " title=" + property.Replace(title) + "::" + data.Replace(ansi.Strip(message))
}

func exitSuffix(r runner.TaskResult) string {
	if code := r.ExitCode(); code > 0 {
		return fmt.Sprintf(" · exit %d", code)
	}
	return ""
}

// formatSeconds formats d as "12.4s" or "2m05s", in parentheses if paren.
func formatSeconds(d time.Duration, paren bool) string {
	s := fmt.Sprintf("%.1fs", d.Seconds())
	if d >= time.Minute {
		d = d.Round(time.Second)
		s = fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if paren {
		return "(" + s + ")"
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// eachLine calls emit with each line of the file at path, splitting it the
// way the live output was, without reading it all into memory.
func eachLine(path string, emit func(line string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var s lineSplitter
	fn := func(l []byte) { emit(string(l)) }
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		s.feed(buf[:n], fn)
		if err == io.EOF {
			s.flush(fn)
			return nil
		}
		if err != nil {
			return err
		}
	}
}
