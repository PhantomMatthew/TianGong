package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/PhantomMatthew/TianGong/internal/agent"
	"github.com/PhantomMatthew/TianGong/internal/config"
	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/storage/sqlc"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Long: `Start an interactive chat session with the TianGong AI agent.

The chat command initializes a conversational session where you can send
messages to the agent and receive streaming responses. The agent has access
to tools (bash, read, write) and can execute commands on your behalf.

Examples:
  # Start a chat session with the default provider
  tg chat

  # Use a specific provider
  tg chat --provider openai

  # Override the model
  tg chat --provider openai --model gpt-4-turbo

  # Resume a previous session
  tg chat --continue session_abc123

Configuration:
  Set up providers via environment variables:
    TIANGONG_PROVIDERS_OPENAI_API_KEY=sk-...
    TIANGONG_PROVIDERS_OPENAI_MODEL=gpt-4o

  Or via config file (./tiangong.yaml, ~/.config/tiangong/tiangong.yaml):
    providers:
      openai:
        api_key: sk-...
        model: gpt-4o

Press Ctrl+C to exit the session.`,
	RunE: runChat,
}

var (
	flagProvider string
	flagModel    string
	flagContinue string
)

func init() {
	chatCmd.Flags().StringVar(&flagProvider, "provider", "", "Provider name (openai, anthropic, google)")
	chatCmd.Flags().StringVar(&flagModel, "model", "", "Model name (overrides provider default)")
	chatCmd.Flags().StringVar(&flagContinue, "continue", "", "Session ID to resume")
}

func runChat(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Select provider
	providerName := flagProvider
	if providerName == "" {
		// Auto-detect first configured provider
		for name, providerCfg := range cfg.Providers {
			if providerCfg.APIKey != "" {
				providerName = name
				break
			}
		}
		if providerName == "" {
			return fmt.Errorf("no provider configured (set TIANGONG_PROVIDERS_<NAME>_API_KEY)")
		}
	}

	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return fmt.Errorf("provider %q not found in configuration", providerName)
	}

	// Override model if flag set
	if flagModel != "" {
		providerCfg.Model = flagModel
	}

	// Create provider
	p, err := provider.NewProvider(providerName, providerCfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Create tool registry and register tools
	tools := tool.NewRegistry()
	if err := tools.Register(tool.NewBash()); err != nil {
		return fmt.Errorf("failed to register bash tool: %w", err)
	}
	if err := tools.Register(tool.NewRead()); err != nil {
		return fmt.Errorf("failed to register read tool: %w", err)
	}
	if err := tools.Register(tool.NewWrite()); err != nil {
		return fmt.Errorf("failed to register write tool: %w", err)
	}

	// Create session store
	var store session.SessionStore
	if cfg.Database.URL != "" {
		conn, err := pgx.Connect(ctx, cfg.Database.URL)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer conn.Close(ctx)

		queries := sqlc.New(conn)
		store = session.NewPostgresStore(queries)
	} else {
		store = session.NewMemoryStore()
	}

	// Create or resume session
	var sess *session.Session
	if flagContinue != "" {
		sess, err = store.GetSession(ctx, flagContinue)
		if err != nil {
			return fmt.Errorf("failed to resume session: %w", err)
		}
		fmt.Printf("Resumed session: %s\n", sess.Title)
	} else {
		sess, err = store.CreateSession(ctx, "Chat Session")
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Create agent
	agentCfg := agent.AgentConfig{}
	if cfg.Agent.MaxIterations > 0 {
		agentCfg.MaxIterations = cfg.Agent.MaxIterations
	}
	if cfg.Agent.HistoryLimit > 0 {
		agentCfg.HistoryLimit = cfg.Agent.HistoryLimit
	}
	if cfg.Agent.SystemPrompt != "" {
		agentCfg.SystemPrompt = cfg.Agent.SystemPrompt
	}

	a := agent.New(p, tools, store, agentCfg)

	// Print welcome banner
	fmt.Printf("TianGong Chat (Provider: %s, Model: %s)\n", p.Name(), providerCfg.Model)
	fmt.Printf("Session ID: %s\n", sess.ID)
	fmt.Println("Type your message and press Enter. Press Ctrl+C to exit.")
	fmt.Println("----------------------------------------")

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\nGoodbye!")
		os.Exit(0)
	}()

	// Interactive loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		err := a.RunStream(ctx, sess.ID, input, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		}
		fmt.Println() // newline after response
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}
