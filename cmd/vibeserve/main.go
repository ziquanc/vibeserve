package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/config"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/snapshot"
	"github.com/vibeserve/vibeserve/internal/store"
	"github.com/vibeserve/vibeserve/internal/tui"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "vibeserve",
		Short: "AI-powered stateful API backend from natural language",
	}

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(routesCmd())
	rootCmd.AddCommand(devCmd())
	rootCmd.AddCommand(undoCmd())

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

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start the API server in interactive REPL mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(configPath, manifestPath, host, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (overrides config)")
	cmd.Flags().StringVar(&host, "host", "", "Server host (overrides config)")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().StringVarP(&configPath, "config", "c", ".vibe/config.yaml", "Path to config.yaml")

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
		return llm.NewOpenAIProvider(apiKey, cfg.Model, cfg.BaseURL), nil
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
		cfg.APIKeyEnv = "ANTHROPIC_API_KEY"
		cfg.Model = "claude-sonnet-4-6-20250514"

		key := prompt("  Anthropic API key: ")
		if key == "" {
			return nil, fmt.Errorf("API key is required")
		}
		os.Setenv("ANTHROPIC_API_KEY", key)

		model := prompt("  Model [claude-sonnet-4-6-20250514]: ")
		if model != "" {
			cfg.Model = model
		}

	case "2":
		cfg.Provider = "openai"

		fmt.Println()
		fmt.Println("  Common endpoints:")
		fmt.Println("    z.ai:    https://open.bigmodel.cn/api/paas/v4/chat/completions")
		fmt.Println("    OpenAI:  https://api.openai.com/v1/chat/completions")
		fmt.Println("    x.ai:    https://api.x.ai/v1/chat/completions")
		fmt.Println("    Groq:    https://api.groq.com/openai/v1/chat/completions")
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
		cfg.APIKeyEnv = "VIBESERVE_API_KEY"
		os.Setenv("VIBESERVE_API_KEY", key)

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

func runDev(configPath, manifestPath, host string, port int) error {
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
			if err := s.Seed(seed.Table, seed.Rows); err != nil {
				return fmt.Errorf("seed %s: %w", seed.Table, err)
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

	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, cfg.Server.CORS)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, handler)

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

	// Start HTTP server in background
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start()
	}()
	log.Printf("Server running at http://%s:%d", cfg.Server.Host, cfg.Server.Port)

	// Create TUI
	serverURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	rootModel := tui.NewRootModel(eng, bus, serverURL)
	p := tea.NewProgram(rootModel)

	// Bridge bus events to TUI
	tui.NewBridge(p, bus)

	// Run TUI (blocks until quit)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
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
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			return fmt.Errorf("seed %s: %w", seed.Table, err)
		}
		log.Printf("Seeded %s: %d rows", seed.Table, len(seed.Rows))
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

	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, true)
	srv := router.NewServer(host, port, handler)

	manifestData, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(".vibe/manifest.json", manifestData, 0o644)

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
