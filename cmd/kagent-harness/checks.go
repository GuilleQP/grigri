package main

import (
	"log/slog"

	"github.com/GuilleQP/grigri/core/belay"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/tool"
)

// newBelayCheckPlugin enforces rule-triggered belay checks: before any tool
// whose name matches rule runs, the belayer reviews the exact call. Only
// climb_on lets the tool run; otherwise the tool is skipped and the climber
// gets the reason as the tool result. The climber cannot opt out.
//
// b may be nil (no belayer bound): matching tools are then always blocked,
// so a missing belayer fails closed.
func newBelayCheckPlugin(rule belay.ToolRule, b *belayer, logger *slog.Logger) (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "grigri-belay-checks",
		BeforeToolCallback: func(ctx agent.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			if !rule.Matches(t.Name()) {
				return nil, nil
			}
			var v belay.Verdict
			if b == nil {
				v = belay.Verdict{Call: belay.OffRoute, Reason: "this tool requires a belay check, but no belayer is configured"}
				logger.Warn("grigri: belay call rejected", "call", belay.WatchMe.Label(), "verdict", v.Call.Label(), "reason", v.Reason, "tool", t.Name())
			} else {
				brief := belay.ToolCallBrief(contentText(ctx.UserContent()), t.Name(), args)
				v = b.check(ctx, belay.WatchMe, brief, "tool", t.Name(), "trigger", "rule")
			}
			if v.Approved() {
				return nil, nil
			}
			// A non-nil result skips the tool; the climber sees this instead.
			return map[string]any{
				"error":    "not run: blocked by a belay check: " + v.Reason,
				"call":     v.Call,
				"approved": false,
				"reason":   v.Reason,
			}, nil
		},
	})
}
