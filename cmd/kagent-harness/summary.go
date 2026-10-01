package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kagent-dev/kagent/go/api/adk"
)

// summarize renders the parts of an AgentConfig that grigri cares about, one
// line per fact, with sub-agents nested below their parent.
func summarize(cfg *adk.AgentConfig) string {
	if cfg == nil {
		return "no AgentConfig received (KAGENT_CONFIG_JSON is empty)\n"
	}
	var b strings.Builder
	writeAgent(&b, cfg, "")
	return b.String()
}

func writeAgent(b *strings.Builder, cfg *adk.AgentConfig, indent string) {
	line := func(format string, args ...any) {
		fmt.Fprintf(b, indent+format+"\n", args...)
	}
	line("name: %s", orNone(cfg.Name))
	line("description: %s", orNone(cfg.Description))
	line("model: %s", modelName(cfg.Model))
	line("instruction: %d chars", len(cfg.Instruction))
	line("tools: http=%d sse=%d stdio=%d remote_agents=%d",
		len(cfg.HttpTools), len(cfg.SseTools), len(cfg.StdioTools), len(cfg.RemoteAgents))
	line("sub_agents: %d", len(cfg.SubAgents))
	for _, sub := range cfg.SubAgents {
		writeAgent(b, sub, indent+"  ")
	}
}

// modelName reads the provider and model through JSON, since each provider has
// its own Go type behind the adk.Model interface.
func modelName(m adk.Model) string {
	if m == nil {
		return "none"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return m.GetType()
	}
	var base struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(raw, &base)
	return strings.TrimSpace(m.GetType() + " " + base.Model)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
