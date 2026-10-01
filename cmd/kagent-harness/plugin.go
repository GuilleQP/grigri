package main

import (
	"log/slog"

	"github.com/GuilleQP/grigri/core/belay"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/tool"
)

// newPlugin returns grigri's ADK plugin. The runner calls it around every model
// and tool call of every agent in the tree (root and sub-agents). For now it
// only logs; budgets, belay checks and the chalk bag will hook in here.
func newPlugin(logger *slog.Logger) (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "grigri",
		BeforeModelCallback: func(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
			logger.Info("grigri: model call", "agent", ctx.AgentName(), "model", req.Model, "contents", len(req.Contents))
			return nil, nil
		},
		AfterModelCallback: func(ctx agent.Context, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
			// When streaming, this runs once per chunk; only the final response
			// carries the whole turn (and is what budgets should count).
			if resp != nil && resp.Partial {
				return nil, nil
			}
			attrs := []any{"agent", ctx.AgentName(), "error", respErr}
			if resp != nil && resp.UsageMetadata != nil {
				attrs = append(attrs,
					"prompt_tokens", resp.UsageMetadata.PromptTokenCount,
					"output_tokens", resp.UsageMetadata.CandidatesTokenCount)
			}
			logger.Info("grigri: model response", attrs...)
			return nil, nil
		},
		BeforeToolCallback: func(ctx agent.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			logger.Info("grigri: tool call", "agent", ctx.AgentName(), "tool", toolLabel(t))
			return nil, nil
		},
		AfterToolCallback: func(ctx agent.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
			logger.Info("grigri: tool result", "agent", ctx.AgentName(), "tool", toolLabel(t), "error", err)
			return nil, nil
		},
	})
}

// toolLabel names a tool in logs; grigri's belay-call tools get their label
// ("🧗 WATCH ME") so they stand out from ordinary tools.
func toolLabel(t tool.Tool) string {
	if c := belay.Call(t.Name()); c.Known() {
		return c.Label()
	}
	return t.Name()
}
