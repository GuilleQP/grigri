// Package belay defines grigri's belay calls: the closed vocabulary that the
// climber, the belayer and the harness use to talk to each other.
//
// The harness decides when a call happens and whether an answer is valid; the
// belayer (an agent) decides which answer to give. Anything that is not a
// valid answer is rejected, and a rejected answer never counts as approval.
//
// Calls travel as identifiers ("off_route"); Spoken returns the climbing
// phrase ("Off route") for logs, prompts and docs.
package belay

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Call is one belay call.
type Call string

const (
	Climbing  Call = "climbing"   // climber: starting a pitch
	WatchMe   Call = "watch_me"   // climber: risky step ahead, review it
	ClimbOn   Call = "climb_on"   // belayer: approved, carry on
	OffRoute  Call = "off_route"  // belayer: blocked or drifting, with a reason
	Take      Call = "take"       // climber: stuck, need help
	Falling   Call = "falling"    // climber: a step failed badly
	LowerMe   Call = "lower_me"   // climber: abandon the pitch
	OffBelay  Call = "off_belay"  // belayer: stop reviewing this pitch
	ToppedOut Call = "topped_out" // climber: pitch done
	BelayOff  Call = "belay_off"  // belayer: confirms the pitch is done
)

var spoken = map[Call]string{
	Climbing:  "Climbing",
	WatchMe:   "Watch me",
	ClimbOn:   "Climb on",
	OffRoute:  "Off route",
	Take:      "Take!",
	Falling:   "Falling!",
	LowerMe:   "Lower me",
	OffBelay:  "Off belay",
	ToppedOut: "Topped out",
	BelayOff:  "Belay off",
}

// Spoken returns the climbing phrase for c, or c itself if it is unknown.
func (c Call) Spoken() string {
	if s, ok := spoken[c]; ok {
		return s
	}
	return string(c)
}

var emoji = map[Call]string{
	Climbing:  "🧗",
	WatchMe:   "🧗",
	ClimbOn:   "🟢",
	OffRoute:  "🔴",
	Take:      "✋",
	Falling:   "⚠️",
	LowerMe:   "🪢",
	OffBelay:  "🔓",
	ToppedOut: "🏔️",
	BelayOff:  "🏁",
}

// Known reports whether c is one of the belay calls.
func (c Call) Known() bool {
	_, ok := spoken[c]
	return ok
}

// Label returns an emoji and the spoken call in capitals, to spot belay calls
// in logs: "🔴 OFF ROUTE". Unknown calls get no emoji.
func (c Call) Label() string {
	label := strings.ToUpper(c.Spoken())
	if e, ok := emoji[c]; ok {
		return e + " " + label
	}
	return label
}

// answers lists the valid belayer answers to each call that expects one.
// Only the calls grigri implements so far are listed.
var answers = map[Call][]Call{
	WatchMe: {ClimbOn, OffRoute},
}

// Answers returns the valid answers to c, or nil if c expects none.
func Answers(c Call) []Call {
	return slices.Clone(answers[c])
}

// Verdict is the belayer's answer to a call.
type Verdict struct {
	Call   Call   `json:"call"`
	Reason string `json:"reason"`
}

// Approved reports whether v lets the climber carry on.
func (v Verdict) Approved() bool {
	return v.Call == ClimbOn
}

// ErrInvalidAnswer wraps every reason an answer is rejected.
var ErrInvalidAnswer = errors.New("invalid belay call")

// ParseAnswer reads the belayer's answer to call from its raw text reply. The
// reply must be a single JSON object {"call": ..., "reason": ...}, optionally
// inside a Markdown code fence, whose call is a valid answer to call and whose
// reason is not empty. Unknown fields are rejected.
func ParseAnswer(call Call, text string) (Verdict, error) {
	valid := answers[call]
	if valid == nil {
		return Verdict{}, fmt.Errorf("%w: %q expects no answer", ErrInvalidAnswer, call)
	}

	dec := json.NewDecoder(strings.NewReader(stripFence(text)))
	dec.DisallowUnknownFields()
	var v Verdict
	if err := dec.Decode(&v); err != nil {
		return Verdict{}, fmt.Errorf("%w: not a JSON belay call: %v", ErrInvalidAnswer, err)
	}
	if dec.More() {
		return Verdict{}, fmt.Errorf("%w: trailing content after the JSON object", ErrInvalidAnswer)
	}
	if !slices.Contains(valid, v.Call) {
		return Verdict{}, fmt.Errorf("%w: %q is not an answer to %q (want one of %v)", ErrInvalidAnswer, v.Call, call, valid)
	}
	v.Reason = strings.TrimSpace(v.Reason)
	if v.Reason == "" {
		return Verdict{}, fmt.Errorf("%w: reason is required", ErrInvalidAnswer)
	}
	return v, nil
}

// stripFence removes surrounding whitespace and one Markdown code fence
// (``` or ```json), which models often add around JSON.
func stripFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimPrefix(text, "json")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	return strings.TrimSpace(text)
}

// WatchMeBrief is the narrow brief the belayer receives for a Watch me call:
// the user's request, the proposed step and the climber's reason, plus the
// exact answer format. The belayer's own prompt (its AgentTemplate) says how
// to judge; this brief says what to judge and how to answer.
func WatchMeBrief(request, step, reason string) string {
	return fmt.Sprintf(`Belay check: the climber called %q before a step.

User request: %s
Proposed step: %s
Climber's reason: %s

Decide whether the climber may take this step. Reply with only this JSON object and nothing else:
{"call": "%s" or "%s", "reason": "<one or two sentences>"}
Use %q to approve and %q to block.`,
		WatchMe.Spoken(), orUnknown(request), orUnknown(step), orUnknown(reason),
		ClimbOn, OffRoute, ClimbOn, OffRoute)
}

func orUnknown(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return "(not given)"
	}
	return s
}
