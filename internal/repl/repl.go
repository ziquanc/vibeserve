// Package repl implements a simple read-eval-print loop for VibeServe.
// Uses plain stdout for scrollable terminal output — no alternate screen.
package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Colors (ANSI)
const (
	reset  = "\033[0m"
	purple = "\033[35m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	cyan   = "\033[36m"
	dim    = "\033[2m"
	bold   = "\033[1m"
	white  = "\033[37m"
	bgDark = "\033[48;5;236m"
)

var version = "0.2.0"

// SetVersion allows main.go to set the version.
func SetVersion(v string) { version = v }

// REPL is the main interactive loop.
type REPL struct {
	engine    *engine.Engine
	bus       *engine.Bus
	serverURL string
	scanner   *bufio.Scanner
	ctx       context.Context
	cancel    context.CancelFunc

	// Streaming progress
	streamingText atomic.Value // stores string

	// State
	pendingSeeds []manifest.Seed
	seedPending  bool
}

// New creates a REPL.
func New(eng *engine.Engine, bus *engine.Bus, serverURL string) *REPL {
	ctx, cancel := context.WithCancel(context.Background())
	r := &REPL{
		engine:    eng,
		bus:       bus,
		serverURL: serverURL,
		scanner:   bufio.NewScanner(os.Stdin),
		ctx:       ctx,
		cancel:    cancel,
	}
	r.streamingText.Store("")
	return r
}

// OnChunk is called by the LLM provider with streaming progress.
// Wire this to provider.OnChunk in main.go.
func (r *REPL) OnChunk(text string) {
	r.streamingText.Store(text)
}

// Run starts the REPL. Blocks until the user quits.
func (r *REPL) Run() error {
	r.printHeader()

	for {
		r.printInputPrompt()

		if !r.scanner.Scan() {
			break
		}

		input := strings.TrimSpace(r.scanner.Text())
		if input == "" {
			continue
		}

		if r.handleInput(input) {
			break
		}
	}

	r.cancel()
	return nil
}

// handleInput processes user input. Returns true if should quit.
func (r *REPL) handleInput(input string) bool {
	// Slash commands — always available
	if strings.HasPrefix(input, "/") {
		return r.handleCommand(input)
	}

	// Seed confirmation
	if r.seedPending {
		r.seedPending = false
		lower := strings.ToLower(input)
		if lower == "y" || lower == "yes" {
			r.applySeedsWithProgress()
		} else {
			r.printInfo("Skipped seeding.")
		}
		r.printURLs()
		return false
	}

	// Blueprint pending — user can approve, reject, or refine
	if r.engine.HasPendingBlueprint() {
		lower := strings.ToLower(input)

		// Approval phrases
		approvalWords := map[string]bool{
			"y": true, "yes": true, "approve": true, "ok": true, "okay": true,
			"good": true, "looks good": true, "lgtm": true, "go": true,
			"proceed": true, "do it": true, "go ahead": true, "ship it": true,
			"confirm": true, "accepted": true, "sure": true, "yep": true,
		}

		// Rejection phrases
		rejectWords := map[string]bool{
			"n": true, "no": true, "cancel": true, "stop": true, "nope": true,
			"reject": true, "discard": true, "nevermind": true, "never mind": true,
		}

		switch {
		case approvalWords[lower]:
			r.approveBlueprint()
		case rejectWords[lower]:
			r.engine.CancelBlueprint()
			r.printInfo("Blueprint cancelled.")
		case lower == "enhance":
			r.refineBlueprint("The current design is too CRUD-heavy. Add state transitions for entities with lifecycle, computed endpoints for analytics, or validation guards for business rules.")
		default:
			// Treat as refinement feedback
			r.refineBlueprint(input)
		}
		return false
	}

	// Regular prompt — send to AI
	r.handlePrompt(input)
	return false
}

func (r *REPL) handleCommand(input string) bool {
	lower := strings.ToLower(input)
	switch {
	case lower == "/quit" || lower == "/exit" || lower == "/q":
		fmt.Println("\nBye!")
		return true
	case lower == "/help" || lower == "/":
		r.printHelp()
	case lower == "/routes":
		r.printRoutes()
	case lower == "/status":
		r.printStatus()
	case lower == "/undo":
		r.handleUndo()
	default:
		fmt.Printf("  %sUnknown command: %s. Type /help for available commands.%s\n", dim, input, reset)
	}
	return false
}

func (r *REPL) handlePrompt(input string) {
	done := r.startProgress("Thinking")

	result, err := r.engine.Apply(r.ctx, input)
	done()

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
	done := r.startProgress("Generating API")

	result, err := r.engine.ApproveBlueprint(r.ctx)
	done()

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
		fmt.Printf("\n  %sSeed sample data? %d tables, %d rows [y/N]%s\n", yellow, len(result.PendingSeeds), totalRows, reset)
	} else {
		r.printURLs()
	}
}

func (r *REPL) refineBlueprint(feedback string) {
	done := r.startProgress("Refining")

	_, err := r.engine.RefineBlueprint(r.ctx, feedback)
	done()

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
		r.printInfo("Undo successful. Last change rolled back.")
	}
}

func (r *REPL) applySeedsWithProgress() {
	for _, seed := range r.pendingSeeds {
		fmt.Printf("  %s✓%s Seeding %s (%d rows)\n", green, reset, seed.Table, len(seed.Rows))
		r.engine.ApplySeeds([]manifest.Seed{seed})
	}
	r.pendingSeeds = nil
	r.printInfo("Data seeded.")
}

// startProgress shows an animated progress indicator with streaming text.
// Returns a function to call when done.
func (r *REPL) startProgress(label string) func() {
	r.streamingText.Store("")
	stopCh := make(chan struct{})
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	go func() {
		i := 0
		for {
			select {
			case <-stopCh:
				return
			default:
				streamText := r.streamingText.Load().(string)
				status := label
				if streamText != "" {
					status = streamText
				}
				fmt.Printf("\r  %s%s %s%s", purple, frames[i%len(frames)], status, reset)
				// Clear rest of line
				fmt.Print("\033[K")
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	return func() {
		close(stopCh)
		fmt.Print("\r\033[K") // Clear the progress line
	}
}

// --- Printing helpers ---

func (r *REPL) printHeader() {
	cwd, _ := os.Getwd()
	cwd = filepath.Base(cwd)

	m := r.engine.Manifest()
	apiName := ""
	tableCount := 0
	routeCount := 0
	if m != nil {
		apiName = m.Name
		tableCount = len(m.Schemas)
		routeCount = len(m.Routes)
	}

	fmt.Println()
	fmt.Printf("  %s%s╭─────────────────────────────────────────────────────────────╮%s\n", bold, purple, reset)
	fmt.Printf("  %s%s│%s  %s%sVibeServe%s %-49s%s%s│%s\n", bold, purple, reset, bold, white, reset, "v"+version, bold, purple, reset)
	fmt.Printf("  %s%s├─────────────────────────────────────────────────────────────┤%s\n", bold, purple, reset)
	fmt.Printf("  %s%s│%s  %sPath:%s      %-47s %s%s│%s\n", bold, purple, reset, dim, reset, cwd, bold, purple, reset)
	fmt.Printf("  %s%s│%s  %sServer:%s    %-47s %s%s│%s\n", bold, purple, reset, dim, reset, r.serverURL, bold, purple, reset)
	if apiName != "" {
		info := fmt.Sprintf("%s (%d tables, %d routes)", apiName, tableCount, routeCount)
		fmt.Printf("  %s%s│%s  %sAPI:%s       %-47s %s%s│%s\n", bold, purple, reset, dim, reset, info, bold, purple, reset)
	}
	fmt.Printf("  %s%s│%s  %sConsole:%s   %-47s %s%s│%s\n", bold, purple, reset, dim, reset, r.serverURL+"/_console", bold, purple, reset)
	fmt.Printf("  %s%s╰─────────────────────────────────────────────────────────────╯%s\n", bold, purple, reset)
	fmt.Println()
	fmt.Printf("  %sType a prompt to create an API, or / to see commands.%s\n\n", dim, reset)
}

func (r *REPL) printInputPrompt() {
	fmt.Printf("  %s───────────────────────────────────────────%s\n", dim, reset)
	fmt.Printf("  %svibe>%s ", purple, reset)
}

func (r *REPL) printBlueprint(bp *engine.BlueprintInfo) {
	fmt.Println()

	// Steps
	if len(bp.Steps) > 0 {
		fmt.Printf("  %s%sBlueprint — %d steps%s\n\n", bold, cyan, len(bp.Steps), reset)
		for i, step := range bp.Steps {
			fmt.Printf("  %s%d.%s %s\n", cyan, i+1, reset, step)
		}
	} else {
		fmt.Printf("  %s%sBlueprint ready%s\n", bold, cyan, reset)
	}

	// Stats
	if bp.Manifest != nil && (len(bp.Manifest.Schemas) > 0 || len(bp.Manifest.Routes) > 0) {
		fmt.Printf("\n  %d tables", len(bp.Manifest.Schemas))
		if len(bp.Manifest.Routes) > 0 {
			fmt.Printf(", %d routes", len(bp.Manifest.Routes))
		}
		if len(bp.Manifest.Scripts) > 0 {
			fmt.Printf(", %d scripts", len(bp.Manifest.Scripts))
		}
		fmt.Println()
	}

	// Warnings
	for _, w := range bp.Warnings {
		fmt.Printf("  %s⚠ %s%s\n", yellow, w, reset)
	}

	// Hints
	if len(bp.Heuristics.Hints) > 0 {
		fmt.Println()
		for _, h := range bp.Heuristics.Hints {
			fmt.Printf("  %s✦ %s%s\n", purple, h, reset)
		}
	}

	// ER Diagram link
	fmt.Printf("\n  %sER Diagram:%s  %s/_blueprint\n", dim, reset, r.serverURL)

	// Approval
	fmt.Printf("\n  %s[y] approve  |  type feedback to refine  |  [n] cancel%s\n", dim, reset)
}

func (r *REPL) printApplyResult(result *engine.ApplyResult) {
	if result == nil {
		return
	}

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

	fmt.Println()
	fmt.Printf("  %s✓ Applied:%s", green, reset)
	parts := []string{}
	if tables > 0 {
		parts = append(parts, fmt.Sprintf("%d schema changes", tables))
	}
	if routes > 0 {
		parts = append(parts, fmt.Sprintf("%d routes", routes))
	}
	if scripts > 0 {
		parts = append(parts, fmt.Sprintf("%d scripts", scripts))
	}
	fmt.Printf(" %s\n", strings.Join(parts, ", "))

	for _, w := range result.Warnings {
		fmt.Printf("  %s⚠ %s%s\n", yellow, w, reset)
	}
}

func (r *REPL) printURLs() {
	fmt.Println()
	fmt.Printf("  %sConsole:%s    %s/_console\n", dim, reset, r.serverURL)
	fmt.Printf("  %sSwagger:%s    %s/_swagger\n", dim, reset, r.serverURL)
	fmt.Printf("  %sBlueprint:%s  %s/_blueprint\n\n", dim, reset, r.serverURL)
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
			fmt.Printf("  %s— %s%s", dim, route.Description, reset)
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
	fmt.Printf("\n  %s%s%s v%s\n", bold, m.Name, reset, m.Version)
	if m.Description != "" {
		fmt.Printf("  %s\n", m.Description)
	}
	fmt.Printf("  Tables: %d  Routes: %d  Scripts: %d\n", len(m.Schemas), len(m.Routes), len(m.Scripts))
	fmt.Printf("  Server: %s\n\n", r.serverURL)
}

func (r *REPL) printHelp() {
	fmt.Println()
	fmt.Printf("  %s%sCommands%s\n\n", bold, cyan, reset)
	fmt.Printf("  %s/routes%s    List all API routes\n", cyan, reset)
	fmt.Printf("  %s/status%s    Show project status\n", cyan, reset)
	fmt.Printf("  %s/undo%s      Rollback last change\n", cyan, reset)
	fmt.Printf("  %s/help%s      Show this help\n", cyan, reset)
	fmt.Printf("  %s/quit%s      Exit VibeServe\n", cyan, reset)
	fmt.Println()
	fmt.Printf("  %sAnything else is sent to the AI to create/modify your API.%s\n\n", dim, reset)
}

func (r *REPL) printAssistant(msg string) {
	fmt.Printf("\n  %sVibeServe:%s %s\n\n", purple, reset, msg)
}

func (r *REPL) printInfo(msg string) {
	fmt.Printf("  %s✓ %s%s\n", green, msg, reset)
}

func (r *REPL) printError(msg string) {
	fmt.Printf("  %s✗ %s%s\n", red, msg, reset)
}
