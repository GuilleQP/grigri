package belay

import (
	"strings"
	"testing"
)

func TestToolRule(t *testing.T) {
	r, err := ParseToolRule(" k8s_delete_*, k8s_scale ,,k8s_patch_?esource ")
	if err != nil {
		t.Fatal(err)
	}
	if r.String() != "k8s_delete_*,k8s_scale,k8s_patch_?esource" {
		t.Errorf("String() = %q", r.String())
	}
	for tool, want := range map[string]bool{
		"k8s_delete_resource":  true,
		"k8s_scale":            true,
		"k8s_patch_resource":   true,
		"k8s_get_resources":    false,
		"k8s_scale_everything": false, // patterns match the whole name
		"watch_me":             false,
	} {
		if got := r.Matches(tool); got != want {
			t.Errorf("Matches(%q) = %v, want %v", tool, got, want)
		}
	}
}

func TestToolRuleEmpty(t *testing.T) {
	for _, spec := range []string{"", " , "} {
		r, err := ParseToolRule(spec)
		if err != nil || !r.Empty() || r.Matches("k8s_delete_resource") {
			t.Errorf("ParseToolRule(%q) = %v, %v; want empty", spec, r, err)
		}
	}
	var zero ToolRule
	if zero.Matches("anything") {
		t.Error("zero ToolRule matches")
	}
}

func TestToolRuleInvalid(t *testing.T) {
	if _, err := ParseToolRule("k8s_[delete"); err == nil {
		t.Error("invalid pattern accepted")
	}
}

func TestToolCallBrief(t *testing.T) {
	brief := ToolCallBrief("scale payments down", "k8s_scale", map[string]any{"name": "payments", "replicas": 0})
	for _, want := range []string{"scale payments down", "Tool: k8s_scale", `"replicas":0`, `"climb_on"`, `"off_route"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief missing %q:\n%s", want, brief)
		}
	}
}
