package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GuilleQP/grigri/core/belay"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

// belayCheckTimeout bounds one belayer run, so a stuck check fails closed.
const belayCheckTimeout = 2 * time.Minute

type watchMeArgs struct {
	Step   string `json:"step" jsonschema:"The exact step you are about to take, e.g. the command or tool call and its target."`
	Reason string `json:"reason" jsonschema:"Why you want to take this step."`
}

type watchMeResult struct {
	Call     belay.Call `json:"call"`
	Approved bool       `json:"approved"`
	Reason   string     `json:"reason"`
}

// belayer runs belay checks. Each check is a fresh, isolated session: the
// belayer sees only the brief, never the climber's conversation.
type belayer struct {
	runner   *runner.Runner
	sessions session.Service
	agent    agent.Agent
	logger   *slog.Logger
}

func newBelayer(a agent.Agent, logPlugin *plugin.Plugin, logger *slog.Logger) (*belayer, error) {
	// During a belay check the belayer only judges; it may not act. kagent
	// gives every agent tools (at least ask_user), so refuse them here.
	noTools, err := plugin.New(plugin.Config{
		Name: "grigri-belay-no-tools",
		BeforeToolCallback: func(ctx agent.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			logger.Info("grigri: belayer tool refused", "tool", t.Name())
			return map[string]any{"error": "tools are not available during a belay check; answer with the belay call JSON"}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	sessions := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "grigri-belay",
		Agent:          a,
		SessionService: sessions,
		PluginConfig:   runner.PluginConfig{Plugins: []*plugin.Plugin{logPlugin, noTools}},
	})
	if err != nil {
		return nil, err
	}
	return &belayer{runner: r, sessions: sessions, agent: a, logger: logger}, nil
}

// check asks the belayer to answer call with brief and validates the answer.
// It never returns an approval it could not validate: any failure is an
// off_route verdict that says what went wrong.
func (b *belayer) check(ctx agent.Context, call belay.Call, brief string) belay.Verdict {
	text, err := b.ask(ctx, brief)
	if err != nil {
		return b.reject(call, fmt.Sprintf("belay check failed: %v", err))
	}
	verdict, err := belay.ParseAnswer(call, text)
	if err != nil {
		return b.reject(call, fmt.Sprintf("the belayer gave no valid answer (%v)", err))
	}
	b.logger.Info("grigri: belay call", "call", call.Label(), "verdict", verdict.Call.Label(), "reason", verdict.Reason)
	return verdict
}

func (b *belayer) reject(call belay.Call, reason string) belay.Verdict {
	b.logger.Warn("grigri: belay call rejected", "call", call.Label(), "verdict", belay.OffRoute.Label(), "reason", reason)
	return belay.Verdict{Call: belay.OffRoute, Reason: reason + "; treat the step as not approved"}
}

// ask runs the belayer once on brief and returns its final text reply.
func (b *belayer) ask(ctx agent.Context, brief string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, belayCheckTimeout)
	defer cancel()

	created, err := b.sessions.Create(runCtx, &session.CreateRequest{
		AppName: "grigri-belay",
		UserID:  ctx.UserID(),
	})
	if err != nil {
		return "", fmt.Errorf("create belay session: %w", err)
	}
	var last string
	for event, err := range b.runner.Run(runCtx, ctx.UserID(), created.Session.ID(), genai.NewContentFromText(brief, genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return "", err
		}
		if event.ErrorMessage != "" {
			return "", fmt.Errorf("belayer error: %s", event.ErrorMessage)
		}
		if event.Author == b.agent.Name() && !event.Partial {
			if text := contentText(event.Content); text != "" {
				last = text
			}
		}
	}
	if last == "" {
		return "", fmt.Errorf("belayer returned no text")
	}
	return last, nil
}

// newWatchMeTool is the climber's watch_me tool: it runs a belay check on the
// proposed step and returns the verdict as the tool result.
func newWatchMeTool(b *belayer) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: string(belay.WatchMe),
		Description: "Ask the belayer to review a risky step before you take it (anything that deletes, " +
			"scales down, restarts or changes production). Returns a verdict: if approved is false, " +
			"do not take the step; tell the user the reason.",
	}, func(ctx agent.Context, args watchMeArgs) (watchMeResult, error) {
		brief := belay.WatchMeBrief(contentText(ctx.UserContent()), args.Step, args.Reason)
		v := b.check(ctx, belay.WatchMe, brief)
		return watchMeResult{Call: v.Call, Approved: v.Approved(), Reason: v.Reason}, nil
	})
}

func contentText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var parts []string
	for _, p := range c.Parts {
		if p != nil && p.Text != "" && !p.Thought {
			parts = append(parts, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
