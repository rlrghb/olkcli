package outfmt

import (
	"strings"
	"testing"
)

type conciseSample struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Body    string `json:"body,omitempty" concise:"omit"`
}

func TestDropConcise_Slice(t *testing.T) {
	in := []conciseSample{
		{ID: "1", Subject: "a", Body: "big body one"},
		{ID: "2", Subject: "b", Body: "big body two"},
	}
	out, ok := dropConcise(in).([]conciseSample)
	if !ok {
		t.Fatalf("dropConcise changed type: %T", dropConcise(in))
	}
	for i := range out {
		if out[i].Body != "" {
			t.Errorf("Body not scrubbed at %d: %q", i, out[i].Body)
		}
	}
	// Input must be untouched (copy semantics).
	if in[0].Body == "" {
		t.Error("dropConcise mutated the caller's slice")
	}
}

func TestDropConcise_StructAndPointer(t *testing.T) {
	s := conciseSample{ID: "1", Subject: "a", Body: "x"}
	if out := dropConcise(s).(conciseSample); out.Body != "" || out.Subject != "a" {
		t.Errorf("struct scrub wrong: %+v", out)
	}
	if out := dropConcise(&s).(*conciseSample); out.Body != "" {
		t.Errorf("pointer scrub wrong: %+v", out)
	}
	if s.Body == "" {
		t.Error("dropConcise mutated the caller's struct")
	}
}

func TestPrintJSON_ConciseDropsBody(t *testing.T) {
	var sb strings.Builder
	p := &Printer{Format: FormatJSON, Writer: &sb, Concise: true}
	if err := p.PrintJSON([]conciseSample{{ID: "1", Subject: "a", Body: "secret big body"}}, 1, ""); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	out := sb.String()
	if strings.Contains(out, "secret big body") {
		t.Errorf("concise output still contains body:\n%s", out)
	}
	if !strings.Contains(out, "\"subject\": \"a\"") {
		t.Errorf("concise output dropped non-omitted field:\n%s", out)
	}
}
