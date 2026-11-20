package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/redbackthomson/nix-tasks/internal/config"
	"github.com/redbackthomson/nix-tasks/internal/tui"
	"github.com/redbackthomson/nix-tasks/internal/ui"
)

// TUICmd launches the interactive TUI
type TUICmd struct{}

// Run executes the TUI command
func (c *TUICmd) Run(globals *Globals) error {
	// Scripts and pipes that run plain nix-tasks get something they can read.
	if !ui.IsTerminal(os.Stdout) {
		return (&ListCmd{}).Run(globals)
	}

	opts, err := tuiOptions(globals)
	if err != nil {
		return err
	}

	if globals.Debug {
		// Log lines written while the TUI owns the screen would corrupt it.
		var logs syncBuffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
		defer func() {
			slog.SetDefault(prev)
			logs.flush(os.Stderr)
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts.Load = func() (*config.Config, error) {
		cfg, _, err := loadConfig(ctx, globals)
		return cfg, err
	}

	return tui.Run(opts)
}

// tuiOptions points the TUI at this nix-tasks binary, rather than whichever
// one is first on PATH, and at the same flake and output flags.
func tuiOptions(globals *Globals) (tui.Options, error) {
	exe, err := os.Executable()
	if err != nil {
		return tui.Options{}, fmt.Errorf("failed to locate the nix-tasks executable: %w", err)
	}
	return tui.Options{
		FlakePath:  globals.Flake,
		Executable: exe,
		Args:       forwardedFlags(globals),
	}, nil
}

func forwardedFlags(globals *Globals) []string {
	args := []string{"-f", globals.Flake}
	if globals.Verbose {
		args = append(args, "-v")
	}
	if globals.Debug {
		args = append(args, "--debug")
	}
	if globals.Color != "" && globals.Color != string(ui.ColorAuto) {
		args = append(args, "--color="+globals.Color)
	}
	return args
}

// syncBuffer collects log output from the goroutine loading the config.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) flush(w io.Writer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.buf.WriteTo(w)
}
