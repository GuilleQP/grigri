// Command kagent-harness runs grigri as a kagent BYO harness.
//
// Adapted from kagent's BYO example (Apache 2.0), go/adk/examples/byo/main.go
// at kagent-dev/kagent@5d192ea1: same pkg/app and ADK executor wiring.
//
// For now it only reports what kagent hands it. At startup it reads the
// compiled AgentConfig (KAGENT_CONFIG_JSON) and Agent Card
// (KAGENT_AGENT_CARD_JSON), and answers (and logs) every message with a
// summary of the config and the environment variable names. It calls no model.
//
// Environment, set by kagent's BYO compiler and the Harness:
//
//   - KAGENT_CONFIG_JSON: the compiled AgentConfig. Credentials appear as
//     __KAGENT_ENV[NAME]__ placeholders, not values.
//   - KAGENT_AGENT_CARD_JSON: the Agent Card kagent advertises.
//   - KAGENT_API_URL: the kagent API endpoint (TaskStore), required by pkg/app.
//   - KAGENT_PORT: the port to listen on; the Harness sets 80 (kagent#2758).
package main

import (
	"encoding/json"
	"iter"
	"log/slog"
	"os"
	"sort"
	"strings"

	a2atype "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/kagent-dev/kagent/go/adk/pkg/app"
	"github.com/kagent-dev/kagent/go/api/adk"
	"github.com/kagent-dev/kagent/go/pkg/logging"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/server/adka2a/v2"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	logger, _ := logging.New(os.Stderr, "info")
	slog.SetDefault(logger)

	cfg, err := loadAgentConfig()
	if err != nil {
		logger.Error("failed to parse KAGENT_CONFIG_JSON", "error", err)
		os.Exit(1)
	}
	card, err := loadAgentCard()
	if err != nil {
		logger.Error("failed to parse KAGENT_AGENT_CARD_JSON", "error", err)
		os.Exit(1)
	}

	report := "AgentConfig received by grigri:\n\n" + summarize(cfg) +
		"\ncredential placeholders: " + strings.Join(placeholderNames(os.Getenv("KAGENT_CONFIG_JSON")), ", ") +
		"\nenvironment variables: " + strings.Join(envNames(), ", ") + "\n"

	reporter, err := adkagent.New(adkagent.Config{
		Name:        "grigri",
		Description: "Replies with a summary of the AgentConfig kagent compiled for it",
		Run: func(ic adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(yield func(*adksession.Event, error) bool) {
				// Logged per turn: startup logs happen while kagent prepares the
				// golden snapshot and never reach an Actor's logs.
				logger.Info("reporting AgentConfig", "report", report)
				event := adksession.NewEvent(ic, ic.InvocationID())
				event.Author = "grigri"
				event.Content = genai.NewContentFromText(report, genai.RoleModel)
				yield(event, nil)
			}
		},
	})
	if err != nil {
		logger.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	executor := adka2a.NewExecutor(adka2a.ExecutorConfig{
		RunnerConfig: runner.Config{
			AppName:        "grigri",
			Agent:          reporter,
			SessionService: adksession.InMemoryService(),
		},
	})

	kagentApp, err := app.New(app.AppConfig{
		AgentCard: card,
		// Port left empty so pkg/app reads KAGENT_PORT (see kagent#2758).
		Logger: logger,
		Agent:  reporter,
	}, executor)
	if err != nil {
		logger.Error("failed to create app", "error", err)
		os.Exit(1)
	}

	if err := kagentApp.Run(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// loadAgentConfig returns nil when kagent supplied no config (e.g. running
// the binary by hand).
func loadAgentConfig() (*adk.AgentConfig, error) {
	raw := strings.TrimSpace(os.Getenv("KAGENT_CONFIG_JSON"))
	if raw == "" {
		return nil, nil
	}
	var cfg adk.AgentConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// loadAgentCard prefers the card kagent advertises and falls back to a minimal
// one, so the binary also starts outside kagent.
func loadAgentCard() (a2atype.AgentCard, error) {
	card := a2atype.AgentCard{
		Name:               "grigri",
		Description:        "grigri BYO harness for kagent",
		Version:            "0.0.0",
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}
	raw := strings.TrimSpace(os.Getenv("KAGENT_AGENT_CARD_JSON"))
	if raw == "" {
		return card, nil
	}
	err := json.Unmarshal([]byte(raw), &card)
	return card, err
}

// envNames lists variable names only; values may hold credentials.
func envNames() []string {
	var names []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
