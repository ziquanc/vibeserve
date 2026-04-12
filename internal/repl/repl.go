// Package repl implements a simple read-eval-print loop for VibeServe.
// Uses plain stdout for scrollable terminal output — no alternate screen.
package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Colors (ANSI)
const (
	colorReset   = "\033[0m"
	colorPurple  = "\033[35m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorRed     = "\033[31m"
	colorCyan    = "\033[36m"
	colorDim     = "\033[2m"
	colorBold    = "\033[1m"
)

// REPL is the main interactive loop.
type REPL struct {
	engine    *engine.Engine
	bus       *engine.Bus
	serverURL string
	scanner   *bufio.Scanner
	ctx       context.Context
	cancel    context.CancelFunc

	// State
	pendingSeeds []manifest.Seed
	seedPending  bool
}

// New creates a REPL.
func New(eng *engine.Engine, bus *engine.Bus, serverURL string) *REPL {
	ctx, cancel := context.WithCancel(context.Background())
	return &REPL{
		engine:    eng,
		bus:       bus,
		serverURL: serverURL,
		scanner:   bufio.NewScanner(os.Stdin),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Run starts the REPL. Blocks until the user quits.
func (r *REPL) Run() error {
	r.printWelcome()

	for {
		prompt := r.getPrompt()
		fmt.Print(prompt)

		if !r.scanner.Scan() {
			break // EOF or error
		}

		input := strings.TrimSpace(r.scanner.Text())
		if input == "" {
			continue
		}

		if r.handleInput(input) {
			break // quit signal
		}
	}

	r.cancel()
	return nil
}

func (r *REPL) getPrompt() string {
	if r.seedPending {
		return colorYellow + "seed> " + colorReset
	}
	if r.engine.HasPendingBlueprint() {
		return colorCyan + "blueprint> " + colorReset
	}
	return colorPurple + "vibe> " + colorReset
}

// handleInput processes user input. Returns true if should quit.
func (r *REPL) handleInput(input string) bool {
	// Seed confirmation
	if r.seedPending {
		r.seedPending = false
		lower := strings.ToLower(input)
		if lower == "y" || lower == "yes" {
			r.applySeedsWithProgress()
		} else {
			r.printSystem("Skipped seeding.")
		}
		r.printURLs()
		return false
	}

	// Blueprint approval
	if r.engine.HasPendingBlueprint() {
		return r.handleBlueprintInput(input)
	}

	// Slash commands
	lower := strings.ToLower(input)
	switch {
	case lower == "/quit" || lower == "/exit":
		fmt.Println("\nBye!")
		return true
	case lower == "/help":
		r.printHelp()
		return false
	case lower == "/routes":
		r.printRoutes()
		return false
	case lower == "/status":
		r.printStatus()
		return false
	case lower == "/undo":
		r.handleUndo()
		return false
	default:
		r.handlePrompt(input)
		return false
	}
}

func (r *REPL) handleBlueprintInput(input string) bool {
	lower := strings.ToLower(input)
	switch {
	case lower == "y" || lower == "yes":
		r.approveBlueprint()
	case lower == "n" || lower == "no" || lower == "/cancel":
		r.engine.CancelBlueprint()
		r.printSystem("Blueprint cancelled.")
	case lower == "enhance":
		r.refineBlueprint("The current design is too CRUD-heavy. Add state transitions for entities with lifecycle, computed endpoints for analytics, or validation guards for business rules.")
	default:
		r.refineBlueprint(input)
	}
	return false
}

func (r *REPL) handlePrompt(input string) {
	r.printThinking("Thinking...")

	result, err := r.engine.Apply(r.ctx, input)
	r.clearLine()

	if err != nil {
		r.printError(err.Error())
		return
	}

	if result.ChatResponse != "" {
		r.printAssistant(result.ChatResponse)
		return
	}

	if result.Blueprint != nil {
		r.printBlueprint(result.Blueprint)
	}
}

func (r *REPL) approveBlueprint() {
	r.printThinking("Generating full API...")

	result, err := r.engine.ApproveBlueprint(r.ctx)
	r.clearLine()

	if err != nil {
		r.printError(err.Error())
		return
	}

	r.printApplyResult(result)

	// Check for pending seeds
	if result != nil && len(result.PendingSeeds) > 0 {
		r.pendingSeeds = result.PendingSeeds
		r.seedPending = true
		totalRows := 0
		for _, s := range result.PendingSeeds {
			totalRows += len(s.Rows)
		}
		fmt.Printf("\n%sSeed sample data? %d tables, %d rows [y/N]%s\n", colorYellow, len(result.PendingSeeds), totalRows, colorReset)
	} else {
		r.printURLs()
	}
}

func (r *REPL) refineBlueprint(feedback string) {
	r.printThinking("Refining blueprint...")

	_, err := r.engine.RefineBlueprint(r.ctx, feedback)
	r.clearLine()

	if err != nil {
		r.printError(err.Error())
		return
	}

	bp := r.engine.PendingBlueprint()
	if bp != nil {
		r.printBlueprint(bp)
	}
}

func (r *REPL) handleUndo() {
	err := r.engine.Undo()
	if err != nil {
		r.printError(fmt.Sprintf("Undo failed: %v", err))
	} else {
		r.printSystem("Undo successful. Last change rolled back.")
	}
}

func (r *REPL) applySeedsWithProgress() {
	for _, seed := range r.pendingSeeds {
		fmt.Printf("  %s✓%s Seeding %s (%d rows)\n", colorGreen, colorReset, seed.Table, len(seed.Rows))
		r.engine.ApplySeeds([]manifest.Seed{seed})
	}
	r.pendingSeeds = nil
	r.printSystem("Data seeded.")
}

// --- Printing helpers ---

func (r *REPL) printWelcome() {
	fmt.Printf("\n%s%sVibeServe%s  %s\n\n", colorBold, colorPurple, colorReset, r.serverURL)
	fmt.Printf("  %sType a prompt to create an API. /help for commands.%s\n\n", colorDim, colorReset)
}

func (r *REPL) printBlueprint(bp *engine.BlueprintInfo) {
	// Steps
	if len(bp.Steps) > 0 {
		fmt.Printf("\n%s%sBlueprint plan: %d steps%s", colorBold, colorCyan, len(bp.Steps), colorReset)
		if bp.Heuristics.Score > 0 {
			fmt.Printf("  %s(score: %d/10)%s", colorDim, bp.Heuristics.Score, colorReset)
		}
		fmt.Println()
		fmt.Println()
		for i, step := range bp.Steps {
			fmt.Printf("  %s%d.%s %s\n", colorCyan, i+1, colorReset, step)
		}
	} else {
		fmt.Printf("\n%s%sBlueprint ready%s", colorBold, colorCyan, colorReset)
		if bp.Heuristics.Score > 0 {
			fmt.Printf("  %s(score: %d/10)%s", colorDim, bp.Heuristics.Score, colorReset)
		}
		fmt.Println()
	}

	// Stats
	if bp.Manifest != nil {
		fmt.Printf("\n  %d tables, %d routes, %d scripts\n",
			len(bp.Manifest.Schemas), len(bp.Manifest.Routes), len(bp.Manifest.Scripts))
	}

	// Warnings
	for _, w := range bp.Warnings {
		fmt.Printf("  %s⚠ %s%s\n", colorYellow, w, colorReset)
	}

	// Hints
	for _, h := range bp.Heuristics.Hints {
		fmt.Printf("  %s✦ %s%s\n", colorPurple, h, colorReset)
	}

	// Suggestions
	for _, s := range bp.Heuristics.Suggestions {
		fmt.Printf("  %s⚠ %s%s\n", colorYellow, s, colorReset)
	}

	// URLs
	fmt.Printf("\n  %sER Diagram:%s  %s/_blueprint\n", colorDim, colorReset, r.serverURL)

	// Approval prompt
	fmt.Printf("\n  %s[y] approve  |  type feedback to refine  |  [n] cancel%s\n\n", colorDim, colorReset)
}

func (r *REPL) printApplyResult(result *engine.ApplyResult) {
	if result == nil {
		return
	}

	// Count changes by type
	tables, routes, scripts := 0, 0, 0
	for _, c := range result.Changes {
		switch {
		case strings.Contains(string(c.Type), "TABLE") || strings.Contains(string(c.Type), "COLUMN"):
			tables++
		case strings.Contains(string(c.Type), "ROUTE"):
			routes++
		case strings.Contains(string(c.Type), "SCRIPT"):
			scripts++
		}
	}

	fmt.Printf("\n%s✓ Applied:%s", colorGreen, colorReset)
	if tables > 0 {
		fmt.Printf(" %d schema changes", tables)
	}
	if routes > 0 {
		fmt.Printf(", %d routes", routes)
	}
	if scripts > 0 {
		fmt.Printf(", %d scripts", scripts)
	}
	fmt.Println()

	for _, w := range result.Warnings {
		fmt.Printf("  %s⚠ %s%s\n", colorYellow, w, colorReset)
	}
}

func (r *REPL) printURLs() {
	fmt.Printf("\n  %sConsole:%s    %s/_console\n", colorDim, colorReset, r.serverURL)
	fmt.Printf("  %sSwagger:%s    %s/_swagger\n", colorDim, colorReset, r.serverURL)
	fmt.Printf("  %sBlueprint:%s  %s/_blueprint\n\n", colorDim, colorReset, r.serverURL)
}

func (r *REPL) printRoutes() {
	m := r.engine.Manifest()
	if m == nil || len(m.Routes) == 0 {
		fmt.Println("  No routes defined yet.")
		return
	}
	fmt.Println()
	for _, route := range m.Routes {
		fmt.Printf("  %-8s %s", route.Method, route.Path)
		if route.Description != "" {
			fmt.Printf("  %s— %s%s", colorDim, route.Description, colorReset)
		}
		fmt.Println()
	}
	fmt.Println()
}

func (r *REPL) printStatus() {
	m := r.engine.Manifest()
	if m == nil {
		fmt.Println("  No API created yet.")
		return
	}
	fmt.Printf("\n  %s%s%s v%s\n", colorBold, m.Name, colorReset, m.Version)
	if m.Description != "" {
		fmt.Printf("  %s\n", m.Description)
	}
	fmt.Printf("  Tables: %d  Routes: %d  Scripts: %d\n", len(m.Schemas), len(m.Routes), len(m.Scripts))
	fmt.Printf("  Server: %s\n\n", r.serverURL)
}

func (r *REPL) printHelp() {
	fmt.Println()
	fmt.Printf("  %s/routes%s    List all API routes\n", colorCyan, colorReset)
	fmt.Printf("  %s/status%s    Show project status\n", colorCyan, colorReset)
	fmt.Printf("  %s/undo%s      Rollback last change\n", colorCyan, colorReset)
	fmt.Printf("  %s/help%s      Show this help\n", colorCyan, colorReset)
	fmt.Printf("  %s/quit%s      Exit VibeServe\n", colorCyan, colorReset)
	fmt.Println()
	fmt.Printf("  %sAnything else is sent to the AI to create/modify your API.%s\n\n", colorDim, colorReset)
}

func (r *REPL) printAssistant(msg string) {
	fmt.Printf("\n%sVibeServe:%s %s\n\n", colorPurple, colorReset, msg)
}

func (r *REPL) printSystem(msg string) {
	fmt.Printf("  %s✓ %s%s\n", colorGreen, msg, colorReset)
}

func (r *REPL) printError(msg string) {
	fmt.Printf("  %s✗ %s%s\n", colorRed, msg, colorReset)
}

func (r *REPL) printThinking(msg string) {
	fmt.Printf("  %s%s%s", colorDim, msg, colorReset)
}

func (r *REPL) clearLine() {
	fmt.Print("\r\033[K")
}
