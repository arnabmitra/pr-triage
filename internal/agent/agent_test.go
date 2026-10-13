package agent

import "testing"

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"routing prefix": "> Auto routed to X\n\n{\"score\":80,\"verdict\":\"got it\"}",
		"fenced":         "Here you go:\n```json\n{\"score\":80,\"verdict\":\"got it\"}\n```\nthanks",
		"bare":           `{"score":80,"verdict":"got it"}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var g Grade
			if err := extractJSON(in, &g); err != nil {
				t.Fatal(err)
			}
			if g.Score != 80 || g.Verdict != "got it" {
				t.Errorf("got %+v", g)
			}
		})
	}
	var g Grade
	if err := extractJSON("no json here", &g); err == nil {
		t.Error("expected error")
	}
}
