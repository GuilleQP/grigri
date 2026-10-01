// Command kagent-harness runs grigri as a kagent BYO harness.
//
// Startup mirrors kagent's own Go runtime (go/adk/cmd/main.go at
// kagent-dev/kagent@5d192ea1, Apache 2.0): kagent's packages turn the compiled
// AgentConfig into Google ADK agents (model, MCP tools, skills, compaction,
// sub-agents) and serve them over A2A. grigri adds its rules as an ADK plugin
// on the runner, so they apply to every agent in the tree.
//
// Not wired yet, compared with kagent's runtime: memory, telemetry export and
// Agent Plugins (skills materialized from plugin packages).
//
// Environment, set by kagent's BYO compiler and the Harness:
//
//   - KAGENT_CONFIG_JSON / KAGENT_AGENT_CARD_JSON: the compiled AgentConfig
//     and Agent Card, written to KAGENT_CONFIG_DIR (default /config).
//   - KAGENT_API_URL: the kagent API endpoint (TaskStore, sessions).
//   - KAGENT_PORT: the port to listen on; the Harness sets 80 (kagent#2758).
package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kagent-dev/kagent/go/adk/pkg/a2a"
	"github.com/kagent-dev/kagent/go/adk/pkg/app"
	"github.com/kagent-dev/kagent/go/adk/pkg/auth"
	"github.com/kagent-dev/kagent/go/adk/pkg/config"
	"github.com/kagent-dev/kagent/go/adk/pkg/controllerclient"
	runnerpkg "github.com/kagent-dev/kagent/go/adk/pkg/runner"
	"github.com/kagent-dev/kagent/go/adk/pkg/session"
	"github.com/kagent-dev/kagent/go/core/pkg/env"
	"github.com/kagent-dev/kagent/go/pkg/logging"
)

func main() {
	logger, err := logging.New(os.Stderr, cmp.Or(env.LogLevel.Get(), env.LogLevel.DefaultValue()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("grigri stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	kagentAPIURL := env.KagentAPIURL.Get()
	if kagentAPIURL == "" {
		return fmt.Errorf("KAGENT_API_URL is required")
	}

	configDir := cmp.Or(env.KagentConfigDir.Get(), env.KagentConfigDir.DefaultValue())
	if err := config.MaterializeFromEnv(configDir); err != nil {
		return fmt.Errorf("materialize agent config in %s: %w", configDir, err)
	}
	agentConfig, agentCard, err := config.LoadAgentConfigs(configDir)
	if err != nil {
		return fmt.Errorf("load agent config from %s: %w", configDir, err)
	}
	// Startup runs while kagent prepares the golden snapshot, so this reaches
	// the logs only when the binary runs outside kagent.
	logger.Info("loaded AgentConfig", "summary", summarize(agentConfig))

	appName := deriveAppName(env.KagentName.Get(), env.KagentNamespace.Get(), agentCard.Name)

	tokenService := auth.NewKAgentTokenService(appName)
	if err := tokenService.Start(context.Background()); err != nil {
		logger.Error("failed to start token service", "error", err)
	}
	defer tokenService.Stop()
	controllerClient, err := controllerclient.New(controllerclient.Config{
		APIURL:        kagentAPIURL,
		AgentName:     appName,
		TokenProvider: tokenService,
	})
	if err != nil {
		return fmt.Errorf("create controller API client for %s: %w", kagentAPIURL, err)
	}
	defer func() {
		if err := controllerClient.Close(); err != nil {
			logger.Error("failed to close controller client", "error", err)
		}
	}()

	sessionService, err := session.NewService(agentConfig.SessionDBURL)
	if err != nil {
		return fmt.Errorf("open session store %s: %w", agentConfig.SessionDBURL, err)
	}

	ctx := logging.IntoContext(context.Background(), logger)
	runnerConfig, err := runnerpkg.CreateRunnerConfig(ctx, agentConfig, sessionService, appName, nil, controllerClient)
	if err != nil {
		return fmt.Errorf("create ADK runner config: %w", err)
	}
	grigriPlugin, err := newPlugin(logger)
	if err != nil {
		return fmt.Errorf("create grigri plugin: %w", err)
	}
	runnerConfig.PluginConfig.Plugins = append(runnerConfig.PluginConfig.Plugins, grigriPlugin)

	stream := agentConfig.GetStream()
	executor, err := a2a.NewKAgentExecutor(a2a.KAgentExecutorConfig{
		RunnerConfig:   runnerConfig,
		SessionService: sessionService,
		Stream:         stream,
		AppName:        appName,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("create A2A executor: %w", err)
	}

	agentCard.Capabilities.Streaming = stream
	kagentApp, err := app.New(app.AppConfig{
		ControllerClient: controllerClient,
		AgentCard:        *agentCard,
		// Port left empty so pkg/app reads KAGENT_PORT (see kagent#2758).
		AppName:         appName,
		ShutdownTimeout: 5 * time.Second,
		Logger:          logger,
		Agent:           runnerConfig.Agent,
	}, executor)
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}
	return kagentApp.Run()
}

// deriveAppName follows kagent's runtime: namespace__NS__name when both are
// set, else the Agent Card name. The BYO compiler sets neither variable today.
func deriveAppName(name, namespace, cardName string) string {
	if name != "" && namespace != "" {
		return strings.ReplaceAll(namespace, "-", "_") + "__NS__" + strings.ReplaceAll(name, "-", "_")
	}
	return cmp.Or(cardName, "grigri")
}
