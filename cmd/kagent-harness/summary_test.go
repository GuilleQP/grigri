package main

import (
	"encoding/json"
	"testing"

	"github.com/kagent-dev/kagent/go/api/adk"
)

func TestSummarize(t *testing.T) {
	raw := `{
		"name": "parent",
		"description": "the parent",
		"instruction": "four",
		"model": {"type": "openai", "model": "gpt-4.1-mini"},
		"sub_agents": [
			{"name": "child", "description": "", "instruction": "", "model": {"type": "openai", "model": "gpt-4.1-mini"}}
		]
	}`
	var cfg adk.AgentConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}

	want := `name: parent
description: the parent
model: openai gpt-4.1-mini
instruction: 4 chars
tools: http=0 sse=0 stdio=0 remote_agents=0
sub_agents: 1
  name: child
  description: none
  model: openai gpt-4.1-mini
  instruction: 0 chars
  tools: http=0 sse=0 stdio=0 remote_agents=0
  sub_agents: 0
`
	if got := summarize(&cfg); got != want {
		t.Errorf("summarize() =\n%s\nwant:\n%s", got, want)
	}
}

func TestSummarizeNil(t *testing.T) {
	if got := summarize(nil); got == "" {
		t.Error("summarize(nil) is empty")
	}
}
