package belay

import (
	"errors"
	"strings"
	"testing"
)

func TestParseAnswer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		want     Verdict
		approved bool
	}{
		{
			name:     "climb on",
			text:     `{"call": "climb_on", "reason": "Staging, low impact."}`,
			want:     Verdict{Call: ClimbOn, Reason: "Staging, low impact."},
			approved: true,
		},
		{
			name: "off route",
			text: `{"call": "off_route", "reason": "Takes payments offline in prod."}`,
			want: Verdict{Call: OffRoute, Reason: "Takes payments offline in prod."},
		},
		{
			name:     "code fence and spaces",
			text:     "\n```json\n{\"call\": \"climb_on\", \"reason\": \"  ok  \"}\n```\n",
			want:     Verdict{Call: ClimbOn, Reason: "ok"},
			approved: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAnswer(WatchMe, tt.text)
			if err != nil {
				t.Fatalf("ParseAnswer() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseAnswer() = %+v, want %+v", got, tt.want)
			}
			if got.Approved() != tt.approved {
				t.Errorf("Approved() = %v, want %v", got.Approved(), tt.approved)
			}
		})
	}
}

func TestParseAnswerRejects(t *testing.T) {
	tests := []struct {
		name string
		call Call
		text string
	}{
		{"free text", WatchMe, "Sure, looks fine!"},
		{"empty", WatchMe, ""},
		{"missing reason", WatchMe, `{"call": "climb_on"}`},
		{"blank reason", WatchMe, `{"call": "climb_on", "reason": "  "}`},
		{"spoken form", WatchMe, `{"call": "Climb on", "reason": "ok"}`},
		{"wrong answer for call", WatchMe, `{"call": "belay_off", "reason": "ok"}`},
		{"unknown call", WatchMe, `{"call": "send_it", "reason": "ok"}`},
		{"unknown field", WatchMe, `{"call": "climb_on", "reason": "ok", "approved": true}`},
		{"two objects", WatchMe, `{"call": "off_route", "reason": "no"} {"call": "climb_on", "reason": "ok"}`},
		{"call expects no answer", Falling, `{"call": "climb_on", "reason": "ok"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAnswer(tt.call, tt.text)
			if !errors.Is(err, ErrInvalidAnswer) {
				t.Fatalf("ParseAnswer() error = %v, want ErrInvalidAnswer", err)
			}
			if got.Approved() {
				t.Errorf("rejected answer is approved: %+v", got)
			}
		})
	}
}

func TestSpoken(t *testing.T) {
	if got := OffRoute.Spoken(); got != "Off route" {
		t.Errorf("OffRoute.Spoken() = %q", got)
	}
	if got := Call("send_it").Spoken(); got != "send_it" {
		t.Errorf("unknown Spoken() = %q", got)
	}
}

func TestLabel(t *testing.T) {
	tests := map[Call]string{
		WatchMe:          "🧗 WATCH ME",
		ClimbOn:          "🟢 CLIMB ON",
		OffRoute:         "🔴 OFF ROUTE",
		Call("send_it"):  "SEND_IT",
		Call("ask_user"): "ASK_USER",
	}
	for call, want := range tests {
		if got := call.Label(); got != want {
			t.Errorf("%q.Label() = %q, want %q", call, got, want)
		}
	}
	for c := range spoken {
		if _, ok := emoji[c]; !ok {
			t.Errorf("%q has no emoji", c)
		}
	}
	if !WatchMe.Known() || Call("ask_user").Known() {
		t.Error("Known() is wrong")
	}
}

func TestAnswers(t *testing.T) {
	got := Answers(WatchMe)
	if len(got) != 2 || got[0] != ClimbOn || got[1] != OffRoute {
		t.Errorf("Answers(WatchMe) = %v", got)
	}
	got[0] = BelayOff // callers get a copy
	if Answers(WatchMe)[0] != ClimbOn {
		t.Error("Answers returned the internal slice")
	}
	if Answers(Falling) != nil {
		t.Error("Answers(Falling) should be nil")
	}
}

func TestWatchMeBrief(t *testing.T) {
	brief := WatchMeBrief("fix payments", "scale payments to 0", "")
	for _, want := range []string{"fix payments", "scale payments to 0", "(not given)", `"climb_on"`, `"off_route"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief missing %q:\n%s", want, brief)
		}
	}
}
