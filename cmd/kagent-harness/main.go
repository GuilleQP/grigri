// Command kagent-harness runs grigri as a kagent BYO harness.
//
// Startup mirrors kagent's own Go runtime (go/adk/cmd/main.go at
// kagent-dev/kagent@5d192ea1, Apache 2.0): kagent's packages turn the compiled
// AgentConfig into Google ADK agents (model, MCP tools, skills, compaction)
// and serve them over A2A. grigri changes two things:
//
//   - The sub-agent bound as "belayer" is not a transfer target. The climber
//     (root) calls it through the watch_me tool, in an isolated session, and
//     gets a validated belay call back (core/belay). Other sub-agents stay
//     ADK sub-agents, as in kagent.
//   - An ADK plugin on the runner sees every model and tool call, and tools
//     matching GRIGRI_BELAY_TOOLS get a belay check before they run.
//
// Not wired yet, compared with kagent's runtime: memory, telemetry export,
// Agent Plugins (skills materialized from plugin packages) and STS token
// propagation.
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

	"github.com/GuilleQP/grigri/core/belay"
	"github.com/kagent-dev/kagent/go/adk/pkg/a2a"
	kagentagent "github.com/kagent-dev/kagent/go/adk/pkg/agent"
	"github.com/kagent-dev/kagent/go/adk/pkg/app"
	"github.com/kagent-dev/kagent/go/adk/pkg/auth"
	"github.com/kagent-dev/kagent/go/adk/pkg/config"
	"github.com/kagent-dev/kagent/go/adk/pkg/controllerclient"
	"github.com/kagent-dev/kagent/go/adk/pkg/session"
	"github.com/kagent-dev/kagent/go/api/adk"
	"github.com/kagent-dev/kagent/go/core/pkg/env"
	"github.com/kagent-dev/kagent/go/pkg/logging"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
)

// belayerName is the subagent binding name grigri treats as the belayer.
const belayerName = "belayer"

// belayToolsEnv lists the tools that need a belay check before they run:
// comma-separated glob patterns, e.g. "k8s_delete_*,k8s_scale". Set it in the
// Harness env. Empty means no rule-triggered checks.
const belayToolsEnv = "GRIGRI_BELAY_TOOLS"

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
	grigriPlugin, err := newPlugin(logger)
	if err != nil {
		return fmt.Errorf("create grigri plugin: %w", err)
	}
	rule, err := belay.ParseToolRule(os.Getenv(belayToolsEnv))
	if err != nil {
		return fmt.Errorf("%s: %w", belayToolsEnv, err)
	}
	logger.Info("belay check rule", "tools", rule.String())
	runnerConfig, err := buildRunnerConfig(ctx, agentConfig, sessionService, appName, rule, grigriPlugin, logger)
	if err != nil {
		return fmt.Errorf("create ADK runner config: %w", err)
	}

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

// buildRunnerConfig replaces kagent's runner.CreateRunnerConfig: same agent
// builder and compaction, but the belayer becomes the watch_me tool instead of
// a transfer target, and tools matching rule get a belay check before they run.
func buildRunnerConfig(ctx context.Context, cfg *adk.AgentConfig, sessions adksession.Service, appName string, rule belay.ToolRule, grigriPlugin *plugin.Plugin, logger *slog.Logger) (runner.Config, error) {
	root := *cfg
	root.SubAgents = nil
	var belayerConfig *adk.AgentConfig
	for _, sub := range cfg.SubAgents {
		if sub.Name == belayerName && belayerConfig == nil {
			belayerConfig = sub
			continue
		}
		root.SubAgents = append(root.SubAgents, sub)
	}

	var extraTools []tool.Tool
	var b *belayer
	if belayerConfig != nil {
		belayerAgent, err := kagentagent.CreateGoogleADKAgent(ctx, belayerConfig, belayerConfig.Name, nil)
		if err != nil {
			return runner.Config{}, fmt.Errorf("create belayer: %w", err)
		}
		b, err = newBelayer(belayerAgent, grigriPlugin, logger)
		if err != nil {
			return runner.Config{}, fmt.Errorf("create belay runner: %w", err)
		}
		watchMe, err := newWatchMeTool(b)
		if err != nil {
			return runner.Config{}, fmt.Errorf("create watch_me tool: %w", err)
		}
		extraTools = append(extraTools, watchMe)
	} else {
		logger.Warn("no subagent named " + belayerName + "; watch_me is disabled")
	}

	rootAgent, err := kagentagent.CreateGoogleADKAgent(ctx, &root, agentNameFromAppName(appName), nil, extraTools...)
	if err != nil {
		return runner.Config{}, fmt.Errorf("create agent: %w", err)
	}
	compaction, err := kagentagent.CompactionConfig(ctx, cfg)
	if err != nil {
		return runner.Config{}, fmt.Errorf("configure context compaction: %w", err)
	}
	plugins := []*plugin.Plugin{grigriPlugin}
	if !rule.Empty() {
		checks, err := newBelayCheckPlugin(rule, b, logger)
		if err != nil {
			return runner.Config{}, fmt.Errorf("create belay check plugin: %w", err)
		}
		plugins = append(plugins, checks)
	}
	return runner.Config{
		AppName:        appName,
		Agent:          rootAgent,
		SessionService: sessions,
		PluginConfig:   runner.PluginConfig{Plugins: plugins},
		Compaction:     compaction,
	}, nil
}

// agentNameFromAppName mirrors kagent's runner: the part after __NS__.
func agentNameFromAppName(appName string) string {
	if i := strings.LastIndex(appName, "__NS__"); i >= 0 {
		return appName[i+len("__NS__"):]
	}
	return appName
}

// deriveAppName follows kagent's runtime: namespace__NS__name when both are
// set, else the Agent Card name. The BYO compiler sets neither variable today.
func deriveAppName(name, namespace, cardName string) string {
	if name != "" && namespace != "" {
		return strings.ReplaceAll(namespace, "-", "_") + "__NS__" + strings.ReplaceAll(name, "-", "_")
	}
	return cmp.Or(cardName, "grigri")
}
