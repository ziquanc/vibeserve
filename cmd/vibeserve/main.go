package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/apitest"
	"github.com/vibeserve/vibeserve/internal/cloud"
	"github.com/vibeserve/vibeserve/internal/config"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/export"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	vibeservemcp "github.com/vibeserve/vibeserve/internal/mcp"
	vibeserverepl "github.com/vibeserve/vibeserve/internal/repl"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/snapshot"
	"github.com/vibeserve/vibeserve/internal/store"
	"github.com/vibeserve/vibeserve/internal/templates"
	"github.com/vibeserve/vibeserve/internal/watch"
	"github.com/vibeserve/vibeserve/internal/web"
)

var version = "0.1.0"

func main() {
	var port int
	var host string
	var manifestPath string
	var configPath string
	var proxyMode bool

	rootCmd := &cobra.Command{
		Use:   "vibeserve",
		Short: "AI-powered stateful API backend from natural language",
		// Default action: run dev mode (no subcommand needed)
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(configPath, manifestPath, host, port, proxyMode)
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (overrides config)")
	rootCmd.Flags().StringVar(&host, "host", "", "Server host (overrides config)")
	rootCmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", ".vibe/config.yaml", "Path to config.yaml")
	rootCmd.Flags().BoolVar(&proxyMode, "proxy", false, "Enable proxy/auto-evolve mode: auto-generate endpoints for unmatched routes")

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(routesCmd())
	rootCmd.AddCommand(devCmd())
	rootCmd.AddCommand(undoCmd())
	rootCmd.AddCommand(exportCmd())
	rootCmd.AddCommand(mcpCmd())
	rootCmd.AddCommand(testCmd())
	rootCmd.AddCommand(diffCmd())
	rootCmd.AddCommand(watchCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(loginCmd(), logoutCmd(), accountCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func upCmd() *cobra.Command {
	var port int
	var host string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the API server from an existing manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(manifestPath, host, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Server port")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("vibeserve", version)
		},
	}
}

func routesCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Print the route table from a manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFromFile(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			for _, r := range m.Routes {
				fmt.Printf("  %-6s %s  → %s\n", r.Method, r.Path, r.Script)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	return cmd
}

func devCmd() *cobra.Command {
	var port int
	var host string
	var manifestPath string
	var configPath string
	var proxyMode bool

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start the API server in interactive REPL mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(configPath, manifestPath, host, port, proxyMode)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (overrides config)")
	cmd.Flags().StringVar(&host, "host", "", "Server host (overrides config)")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().StringVarP(&configPath, "config", "c", ".vibe/config.yaml", "Path to config.yaml")
	cmd.Flags().BoolVar(&proxyMode, "proxy", false, "Enable proxy/auto-evolve mode")

	return cmd
}

func undoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Restore the most recent database snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := snapshot.RestoreLatest(".vibe", ".vibe/state.db")
			if err != nil {
				return fmt.Errorf("undo failed: %w", err)
			}
			fmt.Printf("Restored snapshot #%d: %s\n", snap.ID, snap.Description)
			fmt.Println("Note: restart the server to apply the restored state.")
			return nil
		},
	}
}

func exportCmd() *cobra.Command {
	var manifestPath string
	var force bool
	var ai bool
	var format string
	var db string
	var typescript bool

	cmd := &cobra.Command{
		Use:   "export [output-dir]",
		Short: "Export a standalone server project from the manifest",
		Long:  "Generate a production-ready project from the current VibeServe manifest. Supports Go (default) and Express.js formats.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(manifestPath, args, force, ai, format, db, typescript)
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing output directory")
	cmd.Flags().BoolVar(&ai, "ai", false, "Use LLM to translate complex Tengo logic")
	cmd.Flags().StringVar(&format, "format", "go", "Export format: go, express, next, or fullstack")
	cmd.Flags().StringVar(&db, "db", "", "Database type: sqlite (default) or postgres")
	cmd.Flags().BoolVar(&typescript, "typescript", false, "Generate TypeScript type definitions")

	return cmd
}

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for AI coding assistants",
		Long:  "Start a Model Context Protocol (MCP) server over stdio. AI tools like Claude Code and Cursor can use this to create, modify, and query VibeServe APIs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return vibeservemcp.Run(".vibe")
		},
	}
}

func testCmd() *cobra.Command {
	var manifestPath string
	var port int

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run auto-generated API tests against the running server",
		Long:  "Reads the manifest and runs CRUD lifecycle tests (create, list, get, update, delete, verify-deleted) for each table against the running VibeServe server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFromFile(manifestPath)
			if err != nil {
				return fmt.Errorf("no manifest found at %s. Run 'vibeserve' first to create your API", manifestPath)
			}

			baseURL := fmt.Sprintf("http://localhost:%d", port)
			fmt.Printf("Running tests against %s...\n\n", baseURL)

			results := apitest.RunAll(m, baseURL)
			passed, failed := apitest.PrintResults(results)

			fmt.Printf("\n%d passed, %d failed\n", passed, failed)
			if failed > 0 {
				return fmt.Errorf("%d tests failed", failed)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Server port to test against")

	return cmd
}

func initCmd() *cobra.Command {
	var templateName string
	var list bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project from a starter template",
		Long:  "Create a new VibeServe project from a pre-built template. No LLM call needed — instant setup.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				fmt.Print("\n  Available templates:\n\n")
				for _, t := range templates.List() {
					fmt.Printf("    %-15s %s\n", t.Name, t.Description)
				}
				fmt.Printf("\n  Usage: vibeserve init --template <name>\n\n")
				return nil
			}

			if templateName == "" {
				fmt.Print("\n  Available templates:\n\n")
				for _, t := range templates.List() {
					fmt.Printf("    %-15s %s\n", t.Name, t.Description)
				}
				fmt.Printf("\n  Usage: vibeserve init --template <name>\n\n")
				return nil
			}

			tmpl, err := templates.Get(templateName)
			if err != nil {
				return err
			}

			// Save manifest
			os.MkdirAll(".vibe", 0o755)
			data, _ := json.MarshalIndent(tmpl.Manifest, "", "  ")
			if err := os.WriteFile(".vibe/manifest.json", data, 0o644); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}

			fmt.Printf("\n  ✓ Initialized %s template\n\n", tmpl.Name)
			fmt.Printf("    %s\n", tmpl.Description)
			fmt.Printf("    %d tables, %d routes, %d scripts\n\n", len(tmpl.Manifest.Schemas), len(tmpl.Manifest.Routes), len(tmpl.Manifest.Scripts))
			fmt.Printf("  Next steps:\n")
			fmt.Printf("    vibeserve              Start interactive mode (modify with AI)\n")
			fmt.Printf("    vibeserve up           Start the API server\n")
			fmt.Printf("    vibeserve export       Export to production code\n\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&templateName, "template", "t", "", "Template name (blog, ecommerce, saas)")
	cmd.Flags().BoolVar(&list, "list", false, "List available templates")

	return cmd
}

func diffCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show current API schema, routes, and scripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFromFile(manifestPath)
			if err != nil {
				return fmt.Errorf("no manifest found at %s. Run 'vibeserve' first to create your API", manifestPath)
			}

			printManifestSummary(m)
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	return cmd
}

func printManifestSummary(m *manifest.Manifest) {
	fmt.Printf("%s v%s\n\n", m.Name, m.Version)

	if len(m.Schemas) > 0 {
		fmt.Printf("Tables (%d):\n", len(m.Schemas))
		for _, s := range m.Schemas {
			fmt.Printf("  + %s (%d columns)\n", s.Table, len(s.Columns))
			for _, c := range s.Columns {
				constraints := ""
				if c.Primary {
					constraints += " PK"
				}
				if c.Auto {
					constraints += " AUTO"
				}
				if c.Required {
					constraints += " NOT NULL"
				}
				if c.Unique {
					constraints += " UNIQUE"
				}
				if c.References != "" {
					constraints += " -> " + c.References
				}
				fmt.Printf("    %-18s %-10s%s\n", c.Name, c.Type, constraints)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("No tables defined.")
	}

	if len(m.Routes) > 0 {
		fmt.Printf("Routes (%d):\n", len(m.Routes))
		for _, r := range m.Routes {
			desc := ""
			if r.Description != "" {
				desc = " — " + r.Description
			}
			fmt.Printf("  + %-8s %-28s%s\n", r.Method, r.Path, desc)
		}
		fmt.Println()
	} else {
		fmt.Println("No routes defined.")
	}

	fmt.Printf("Scripts: %d total\n", len(m.Scripts))
	if len(m.Seeds) > 0 {
		totalRows := 0
		for _, s := range m.Seeds {
			totalRows += len(s.Rows)
		}
		fmt.Printf("Seeds: %d tables, %d rows\n", len(m.Seeds), totalRows)
	}
}

func runExport(manifestPath string, args []string, force, ai bool, format, db string, typescript bool) error {
	// Load manifest
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("no manifest found at %s. Run 'vibeserve' first to create your API", manifestPath)
	}

	// Determine output directory
	var outDir string
	if len(args) > 0 {
		outDir = args[0]
	} else {
		outDir = export.DefaultOutputDir(m)
	}

	// Check if directory exists
	if info, err := os.Stat(outDir); err == nil && info.IsDir() {
		if !force {
			fmt.Printf("Directory %q already exists. Overwrite? [y/N] ", outDir)
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("Export cancelled.")
				return nil
			}
		}
		os.RemoveAll(outDir)
	}

	// Validate format
	switch format {
	case "go", "express", "next", "fullstack":
		// valid
	default:
		return fmt.Errorf("unsupported format %q (supported: go, express, next, fullstack)", format)
	}

	fmt.Printf("Exporting to %s (format: %s)...\n", outDir, format)

	// Run export pipeline
	exp := export.NewExporter(m, outDir)

	if db != "" {
		exp.SetDBType(db)
	}
	if typescript {
		exp.SetTypeScript(true)
	}

	var exportErr error
	switch format {
	case "express":
		exportErr = exp.RunExpress()
	case "next":
		exportErr = exp.RunNext()
	case "fullstack":
		exportErr = exp.RunFullstack()
	default:
		exportErr = exp.Run()
	}
	if exportErr != nil {
		return fmt.Errorf("export failed: %w", exportErr)
	}

	if format == "go" {
		// Post-process: go mod tidy
		fmt.Println("Running go mod tidy...")
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = outDir
		if tidyOut, err := tidyCmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: go mod tidy failed: %s\n", string(tidyOut))
		}

		// Post-process: gofmt
		fmtCmd := exec.Command("gofmt", "-w", ".")
		fmtCmd.Dir = outDir
		fmtCmd.Run()
	}

	// Success banner
	fmt.Println()
	fmt.Println("  \u2713 Export complete!")
	fmt.Println()
	switch format {
	case "express":
		fmt.Printf("  Your production Express.js server is ready at: ./%s\n", outDir)
		fmt.Println()
		if exp.DBType() == "postgres" {
			fmt.Println("  Database setup:")
			fmt.Printf("    psql -d your_database -f %s/schema.sql\n", outDir)
			fmt.Printf("    psql -d your_database -f %s/seed.sql\n", outDir)
			fmt.Println()
		}
		fmt.Println("  To start:")
		fmt.Printf("    cd %s\n", outDir)
		fmt.Println("    npm install")
		fmt.Println("    npm start")
	case "next":
		fmt.Printf("  Your Next.js admin panel is ready at: ./%s\n", outDir)
		fmt.Println()
		fmt.Println("  To start:")
		fmt.Printf("    cd %s\n", outDir)
		fmt.Println("    npm install")
		fmt.Println("    npm run dev")
		fmt.Println()
		fmt.Println("  Make sure your VibeServe server is running at localhost:8080")
	case "fullstack":
		fmt.Printf("  Your full-stack app is ready at: ./%s\n", outDir)
		fmt.Println()
		fmt.Printf("    %s/backend/   Express.js API\n", outDir)
		fmt.Printf("    %s/frontend/  Next.js admin panel\n", outDir)
		fmt.Println()
		fmt.Println("  To start:")
		fmt.Printf("    cd %s\n", outDir)
		fmt.Println("    npm run install:all")
		fmt.Println("    npm run dev")
	default:
		fmt.Printf("  Your production Go server is ready at: ./%s\n", outDir)
		fmt.Println()
		fmt.Println("  To start:")
		fmt.Printf("    cd %s\n", outDir)
		fmt.Println("    go run ./cmd/api")
	}
	fmt.Println()
	fmt.Println("  Check README.md for API documentation.")
	fmt.Println()

	return nil
}

func createProvider(cfg *config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case "claude":
		apiKey := cfg.APIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("provider 'claude' requires %s environment variable to be set", cfg.APIKeyEnv)
		}
		return llm.NewClaudeProvider(apiKey, cfg.Model), nil
	case "ollama":
		return llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model), nil
	case "openai":
		apiKey := cfg.APIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("provider 'openai' requires %s environment variable to be set", cfg.APIKeyEnv)
		}
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider 'openai' requires base_url in config")
		}
		var opts []llm.OpenAIOption
		if cfg.MaxTokens > 0 {
			opts = append(opts, llm.WithOpenAIMaxTokens(cfg.MaxTokens))
		}
		return llm.NewOpenAIProvider(apiKey, cfg.Model, cfg.BaseURL, opts...), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: claude, openai, ollama)", cfg.Provider)
	}
}

// runSetup runs an interactive first-time setup wizard.
// Returns the config path where the config was saved.
func runSetup(configPath string) (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)
	prompt := func(msg string) string {
		fmt.Print(msg)
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line)
	}

	fmt.Println()
	fmt.Println("  Welcome to VibeServe!")
	fmt.Println("  Let's set up your AI provider.")
	fmt.Println()
	fmt.Println("  [1] Claude (Anthropic)")
	fmt.Println("  [2] OpenAI-compatible (z.ai, x.ai, Groq, Together, etc.)")
	fmt.Println("  [3] Ollama (local, no API key needed)")
	fmt.Println()

	choice := prompt("  Choose provider [1/2/3]: ")

	cfg := config.DefaultConfig()

	switch choice {
	case "1", "":
		cfg.Provider = "claude"
		cfg.APIKeyEnv = ""
		cfg.Model = "claude-sonnet-4-6-20250514"

		key := prompt("  Anthropic API key: ")
		if key == "" {
			return nil, fmt.Errorf("API key is required")
		}
		cfg.APIKeyVal = key

		model := prompt("  Model [claude-sonnet-4-6-20250514]: ")
		if model != "" {
			cfg.Model = model
		}

	case "2":
		cfg.Provider = "openai"

		fmt.Println()
		fmt.Println("  Common endpoints (/chat/completions is auto-appended):")
		fmt.Println("    z.ai:    https://open.bigmodel.cn/api/paas/v4")
		fmt.Println("    OpenAI:  https://api.openai.com/v1")
		fmt.Println("    x.ai:    https://api.x.ai/v1")
		fmt.Println("    Groq:    https://api.groq.com/openai/v1")
		fmt.Println()

		baseURL := prompt("  API endpoint URL: ")
		if baseURL == "" {
			return nil, fmt.Errorf("endpoint URL is required")
		}
		cfg.BaseURL = baseURL

		key := prompt("  API key: ")
		if key == "" {
			return nil, fmt.Errorf("API key is required")
		}
		cfg.APIKeyEnv = ""
		cfg.APIKeyVal = key

		model := prompt("  Model name: ")
		if model == "" {
			return nil, fmt.Errorf("model name is required")
		}
		cfg.Model = model

	case "3":
		cfg.Provider = "ollama"
		cfg.OllamaHost = "http://localhost:11434"

		host := prompt("  Ollama host [http://localhost:11434]: ")
		if host != "" {
			cfg.OllamaHost = host
		}

		model := prompt("  Model name [llama3]: ")
		if model == "" {
			model = "llama3"
		}
		cfg.Model = model

	default:
		return nil, fmt.Errorf("invalid choice: %q", choice)
	}

	// Save config
	if err := cfg.Save(configPath); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Config saved to %s\n", configPath)
	fmt.Println("  Starting VibeServe...")
	fmt.Println()

	return cfg, nil
}

func runDev(configPath, manifestPath, host string, port int, proxyMode bool) error {
	// Check if config exists — if not, run interactive setup
	var cfg *config.Config
	var err error

	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		cfg, err = runSetup(configPath)
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
	} else {
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
	}

	// Flag overrides take precedence over config
	if host != "" {
		cfg.Server.Host = host
	}
	if port != 0 {
		cfg.Server.Port = port
	}

	provider, err := createProvider(cfg)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}

	// Load or start fresh manifest
	var m *manifest.Manifest
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		m, err = manifest.LoadFromFile(manifestPath)
		if err != nil {
			return fmt.Errorf("load manifest: %w", err)
		}
		log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))
	} else {
		m = nil
		log.Println("No manifest found; starting fresh")
	}

	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	os.MkdirAll(".vibe", 0o755)

	s, err := store.New(".vibe/state.db")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if m != nil {
		if err := s.ApplySchemas(m.Schemas); err != nil {
			return fmt.Errorf("apply schemas: %w", err)
		}
		for _, seed := range m.Seeds {
			count, _ := s.CountAll(seed.Table)
			if count == 0 {
				if err := s.Seed(seed.Table, seed.Rows); err != nil {
					return fmt.Errorf("seed %s: %w", seed.Table, err)
				}
			}
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	if m != nil {
		for _, sc := range m.Scripts {
			scripts[sc.Name] = sc.Code
		}
		for _, r := range m.Routes {
			trie.Insert(r.Method, r.Path, r.Script)
		}
	}

	rt := runtime.New(s, bus, cfg.JWTSecret)

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: m,
		VibeDir:  ".vibe",
		StoreOpener: func(dsn string) (engine.SchemaStore, error) {
			return store.New(dsn)
		},
	})
	eng.LoadHistory() // Restore conversation context from previous sessions

	var apiHandler http.Handler
	if proxyMode {
		proxyEng := engine.NewProxyEngine(eng)
		apiHandler = router.NewProxyHandler(trie, eng.GetScript, rt, cfg.Server.CORS, proxyEng.HandleUnknownRequest)
		log.Printf("Proxy/auto-evolve mode enabled — unmatched routes will be auto-generated")
	} else {
		apiHandler = router.NewHandler(trie, eng.GetScript, rt, cfg.Server.CORS)
	}

	consoleHandler := web.NewConsole(eng, s)
	wsHub := web.NewWSHub(bus)
	blueprintHandler := web.NewBlueprintHandler(eng)
	mux := web.NewConsoleMux(apiHandler, consoleHandler, wsHub, blueprintHandler)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, mux)

	// Start HTTP server in background
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start()
	}()
	log.Printf("Server running at http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Console at http://%s:%d/_console/", cfg.Server.Host, cfg.Server.Port)

	// Start REPL (plain terminal — scrollable, no AltScreen)
	serverURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	r := vibeserverepl.New(eng, bus, serverURL)

	// Wire streaming callback for live progress
	if openaiP, ok := provider.(*llm.OpenAIProvider); ok {
		openaiP.OnChunk = r.OnChunk
	}

	if err := r.Run(); err != nil {
		return fmt.Errorf("REPL error: %w", err)
	}

	// Cleanup
	srv.Shutdown(context.Background())
	return nil
}

func runUp(manifestPath, host string, port int) error {
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))

	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	log.Println("Manifest validated")

	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	os.MkdirAll(".vibe", 0o755)

	s, err := store.New(".vibe/state.db")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.ApplySchemas(m.Schemas); err != nil {
		return fmt.Errorf("apply schemas: %w", err)
	}
	for _, schema := range m.Schemas {
		log.Printf("Schema applied: %s (%d columns)", schema.Table, len(schema.Columns))
	}

	for _, seed := range m.Seeds {
		count, _ := s.CountAll(seed.Table)
		if count == 0 {
			if err := s.Seed(seed.Table, seed.Rows); err != nil {
				return fmt.Errorf("seed %s: %w", seed.Table, err)
			}
			log.Printf("Seeded %s: %d rows", seed.Table, len(seed.Rows))
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
		log.Printf("Route registered: %s %s → %s", r.Method, r.Path, r.Script)
	}

	cfg, _ := config.Load(".vibe/config.yaml")
	rt := runtime.New(s, bus, cfg.JWTSecret)

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Manifest: m,
		VibeDir:  ".vibe",
		StoreOpener: func(dsn string) (engine.SchemaStore, error) {
			return store.New(dsn)
		},
	})

	apiHandler := router.NewHandler(trie, eng.GetScript, rt, true)

	consoleHandler := web.NewConsole(eng, s)
	wsHub := web.NewWSHub(bus)
	blueprintHandler := web.NewBlueprintHandler(eng)
	mux := web.NewConsoleMux(apiHandler, consoleHandler, wsHub, blueprintHandler)
	srv := router.NewServer(host, port, mux)

	manifestData, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(".vibe/manifest.json", manifestData, 0o644)

	log.Printf("Console at http://%s:%d/_console/", host, port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		log.Println("Shutting down...")
		return srv.Shutdown(context.Background())
	}
}

// Watch mode color constants.
const (
	watchBold   = "\033[1m"
	watchPurple = "\033[35m"
	watchGreen  = "\033[32m"
	watchRed    = "\033[31m"
	watchDim    = "\033[2m"
	watchReset  = "\033[0m"
)

func watchCmd() *cobra.Command {
	var manifestPath string
	var exportDir string
	var format string
	var db string
	var typescript bool
	var port int
	var host string

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Run API server and auto re-export on manifest changes",
		Long:  "Starts the API server and watches the manifest file. When it changes (via chat, MCP, or direct edit), automatically re-exports the project and generates migration SQL.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if exportDir == "" {
				return fmt.Errorf("--export flag is required")
			}
			return runWatch(manifestPath, exportDir, format, db, typescript, host, port)
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().StringVar(&exportDir, "export", "", "Path to exported project directory (required)")
	cmd.Flags().StringVar(&format, "format", "express", "Export format: go, express, next, or fullstack")
	cmd.Flags().StringVar(&db, "db", "sqlite", "Database type: sqlite or postgres")
	cmd.Flags().BoolVar(&typescript, "typescript", false, "Generate TypeScript type definitions")
	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Server port")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")

	return cmd
}

func runWatch(manifestPath, exportDir, format, dbType string, typescript bool, host string, port int) error {
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Same server setup as runUp
	bus := engine.NewBus()
	os.MkdirAll(".vibe", 0o755)
	s, err := store.New(".vibe/state.db")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.ApplySchemas(m.Schemas); err != nil {
		return fmt.Errorf("apply schemas: %w", err)
	}
	for _, seed := range m.Seeds {
		count, _ := s.CountAll(seed.Table)
		if count == 0 {
			_ = s.Seed(seed.Table, seed.Rows)
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
	}

	cfg, _ := config.Load(".vibe/config.yaml")
	rt := runtime.New(s, bus, cfg.JWTSecret)
	eng := engine.NewEngine(engine.EngineConfig{
		Bus: bus, Store: s, Trie: trie, Scripts: scripts,
		Manifest: m, VibeDir: ".vibe",
		StoreOpener: func(dsn string) (engine.SchemaStore, error) { return store.New(dsn) },
	})

	apiHandler := router.NewHandler(trie, eng.GetScript, rt, false)
	consoleHandler := web.NewConsole(eng, s)
	wsHub := web.NewWSHub(bus)
	blueprintHandler := web.NewBlueprintHandler(eng)
	mux := web.NewConsoleMux(apiHandler, consoleHandler, wsHub, blueprintHandler)
	srv := router.NewServer(host, port, mux)

	// Initial export
	fmt.Printf("\n  %s%sVibeServe Watch%s\n\n", watchBold, watchPurple, watchReset)
	fmt.Printf("  Server:    http://%s:%d\n", host, port)
	fmt.Printf("  Export:    %s (format: %s)\n", exportDir, format)
	fmt.Printf("  Watching:  %s\n\n", manifestPath)

	// Do initial export
	{
		exp := export.NewExporter(m, exportDir)
		exp.SetVibeDir(".vibe")
		exp.SetDBType(dbType)
		if typescript {
			exp.SetTypeScript(true)
		}
		switch format {
		case "next":
			err = exp.RunNext()
		case "express":
			err = exp.RunExpress()
		default:
			err = exp.Run()
		}
		if err != nil {
			fmt.Printf("  %sx Initial export failed: %v%s\n", watchRed, err, watchReset)
		} else {
			fmt.Printf("  %sv Initial export complete%s\n", watchGreen, watchReset)
		}
	}

	// Start watcher
	watcher := watch.New(watch.Config{
		ManifestPath: manifestPath,
		ExportDir:    exportDir,
		Format:       format,
		DBType:       dbType,
		TypeScript:   typescript,
	})

	stopWatch := make(chan struct{})
	go watcher.Start(stopWatch, func(summary string) {
		fmt.Printf("\n  %sv Manifest changed%s\n%s\n\n", watchGreen, watchReset, summary)
	})

	// Start server
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	fmt.Printf("  %sv Server running at http://%s:%d%s\n", watchGreen, host, port, watchReset)
	fmt.Printf("  %sWaiting for manifest changes...%s\n\n", watchDim, watchReset)

	select {
	case err := <-errCh:
		close(stopWatch)
		return err
	case <-sigCh:
		close(stopWatch)
		srv.Shutdown(context.Background())
		fmt.Println("\n  Shutting down...")
		return nil
	}
}

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to your VibeServe account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cloud.IsLoggedIn() {
				creds := cloud.LoadCredentials()
				fmt.Printf("\n  Already logged in as %s (%s plan)\n", creds.Email, creds.Plan)
				fmt.Printf("  Run 'vibeserve logout' to switch accounts.\n\n")
				return nil
			}
			_, err := cloud.Login()
			return err
		},
	}
}

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of your VibeServe account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cloud.IsLoggedIn() {
				fmt.Print("\n  Not logged in.\n\n")
				return nil
			}
			cloud.DeleteCredentials()
			fmt.Print("\n  Logged out.\n\n")
			return nil
		},
	}
}

func accountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Show your VibeServe account info",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds := cloud.LoadCredentials()
			if creds == nil {
				fmt.Print("\n  Not logged in. Run 'vibeserve login' to connect.\n\n")
				return nil
			}
			fmt.Printf("\n  Email: %s\n  Plan:  %s\n\n", creds.Email, creds.Plan)
			return nil
		},
	}
}
