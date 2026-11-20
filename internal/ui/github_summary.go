package ui

import (
	_ "embed"
	"fmt"
	"html"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

// MaxJobSummaryBytes is GitHub's limit on the job summary of one step.
const MaxJobSummaryBytes = 1 << 20

const summaryTruncatedNote = "\n_The summary was cut short to fit GitHub's 1 MiB limit; the job log has every task._\n"

// writeJobSummary appends the run's markdown summary to SummaryPath.
func (g *GitHubReporter) writeJobSummary(results []runner.TaskResult, elapsed time.Duration) error {
	limit := MaxJobSummaryBytes
	if info, err := os.Stat(g.opts.SummaryPath); err == nil {
		limit -= int(info.Size())
	}
	if limit <= 0 {
		return nil
	}

	var details []string
	for _, r := range results {
		if failed(r) {
			details = append(details, g.summaryDetails(r))
		}
	}
	md := fitSummary(summaryTable(strings.Join(g.plan.Targets, " "), results, g.warnings, elapsed), details, limit)

	f, err := os.OpenFile(g.opts.SummaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(md)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// fitSummary joins the table and as many details blocks as fit in limit
// bytes, noting when anything was left out.
func fitSummary(table string, details []string, limit int) string {
	if len(table)+len(summaryTruncatedNote) > limit {
		cut := table[:max(limit-len(summaryTruncatedNote), 0)]
		if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
			cut = cut[:i+1]
		}
		return cut + summaryTruncatedNote
	}
	var b strings.Builder
	b.WriteString(table)
	for _, d := range details {
		if b.Len()+len(d)+len(summaryTruncatedNote) > limit {
			b.WriteString(summaryTruncatedNote)
			break
		}
		b.WriteString(d)
	}
	return b.String()
}

type summaryRow struct {
	Name, Result, Notes string
	Duration            time.Duration
	// Timed is unset for cached and skipped tasks, which show no duration.
	Timed bool
}

type summaryCount struct{ Kind, Text string }

type summaryTableData struct {
	Target, Marker string
	Rows           []summaryRow
	Totals         []summaryCount
	Elapsed        time.Duration
	Warnings       []string
}

type summaryDetailsData struct {
	Marker, Name, Fence, Body string
	Cut                       bool
	Shown                     int
}

func summaryTable(target string, results []runner.TaskResult, warnings []string, elapsed time.Duration) string {
	c := countResults(results)
	rows := make([]summaryRow, 0, len(results))
	for _, r := range results {
		row := summaryRow{Name: r.Name, Duration: r.Duration, Timed: true}
		switch {
		case r.State == runner.Succeeded && r.Cached:
			row.Result, row.Timed, row.Notes = "● cached", false, strings.TrimPrefix(savedSuffix(r, " "), " ")
		case r.State == runner.Succeeded:
			row.Result = "✓ passed"
			if r.Cache != nil && r.Cache.Uncached != "" {
				row.Notes = "not cached: " + r.Cache.Uncached
			}
		case r.State == runner.Failed:
			row.Result, row.Notes = "✗ failed", failureNote(r)
		case r.State == runner.FailedAllowed:
			row.Result, row.Notes = "! failed (allowed)", failureNote(r)
		case r.State == runner.Skipped:
			row.Result, row.Timed, row.Notes = "- skipped", false, r.SkipReason
		default:
			row.Result = "⏹ interrupted"
		}
		for _, w := range r.Warnings {
			row.Notes = joinNote(row.Notes, w)
		}
		rows = append(rows, row)
	}
	var totals []summaryCount
	c.parts(func(kind, s string) string {
		totals = append(totals, summaryCount{kind, s})
		return s
	})
	return renderSummary("table", summaryTableData{target, c.marker(), rows, totals, elapsed, warnings})
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func failureNote(r runner.TaskResult) string {
	if code := r.ExitCode(); code > 0 {
		return fmt.Sprintf("exit %d", code)
	}
	return errorDetail(r)
}

// summaryDetails is a collapsible block with the end of a failed task's
// output.
func (g *GitHubReporter) summaryDetails(r runner.TaskResult) string {
	var lines []string
	total := 0
	if t := g.tasks[r.Name]; t != nil && t.path != "" {
		_ = eachLine(t.path, func(line string) {
			total++
			if len(lines) == FailedOutputTailLines {
				copy(lines, lines[1:])
				lines = lines[:len(lines)-1]
			}
			lines = append(lines, g.redact(ansi.Strip(line)))
		})
	}
	if detail := errorDetail(r); detail != "" {
		lines = append(lines, "error: "+detail)
	}

	d := summaryDetailsData{
		Marker: "✗",
		Name:   r.Name,
		Fence:  "```",
		Body:   strings.Join(lines, "\n"),
		Cut:    total > FailedOutputTailLines,
		Shown:  FailedOutputTailLines,
	}
	if r.State == runner.FailedAllowed {
		d.Marker = "!"
	}
	for strings.Contains(d.Body, d.Fence) {
		d.Fence += "`"
	}
	return renderSummary("details", d)
}

//go:embed github_summary.md.tmpl
var summaryTemplateText string

var summaryTemplate = template.Must(template.New("summary").Funcs(template.FuncMap{
	"cell":    mdCell,
	"escape":  html.EscapeString,
	"seconds": func(d time.Duration) string { return formatSeconds(d, false) },
}).Parse(summaryTemplateText))

// renderSummary panics on error: the template is fixed, so an error is a bug
// the golden tests catch.
func renderSummary(name string, data any) string {
	var b strings.Builder
	if err := summaryTemplate.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Sprintf("job summary template %s: %v", name, err))
	}
	return b.String()
}

// mdCell makes s safe inside a markdown table cell.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return html.EscapeString(s)
}
