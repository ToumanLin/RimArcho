// Package finding is the problem (Finding) model of 03 §4: code, severity, subjects,
// claim, evidence, confidence and suggestions. Instance references are stored as
// "@{<instance id>}" tokens and turned into short refs when rendered.
package finding

import (
	"sort"
	"strings"
)

// Severities.
const (
	Error   = "error"
	Warning = "warning"
	Info    = "info"
)

// Confidence levels.
const (
	Certain  = "certain"
	Likely   = "likely"
	Possible = "possible"
)

// Evidence source types (03 §4.1).
const (
	Declared  = "declared"
	Inferred  = "inferred"
	Log       = "log"
	Community = "community"
	User      = "user"
)

// Evidence is one supporting fact.
type Evidence struct {
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Text   string `json:"text"`
}

// Finding is one problem.
type Finding struct {
	Code        string     `json:"code"`
	Severity    string     `json:"severity"`
	Subjects    []string   `json:"subjects"` // instance tokens or other refs; the first is the subject
	Claim       string     `json:"claim"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	Confidence  string     `json:"confidence"`
	Suggestions []string   `json:"suggestions,omitempty"`
	// Key identifies the finding across runs (for ignores); unique per code.
	Key string `json:"key"`
	// Layer: decl, def or patch.
	Layer string `json:"layer"`
}

// Inst is the token for an instance reference.
func Inst(id string) string { return "@{" + id + "}" }

// InstID extracts the instance ID from a token ("" if it is not one).
func InstID(tok string) string {
	if strings.HasPrefix(tok, "@{") && strings.HasSuffix(tok, "}") {
		return tok[2 : len(tok)-1]
	}
	return ""
}

// Expand replaces every "@{id}" token in s using ref.
func Expand(s string, ref func(id string) string) string {
	for {
		i := strings.Index(s, "@{")
		if i < 0 {
			return s
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			return s
		}
		s = s[:i] + ref(s[i+2:i+j]) + s[i+j+1:]
	}
}

// SubjectInstance is the instance ID of the first instance subject ("" if none).
func (f *Finding) SubjectInstance() string {
	for _, s := range f.Subjects {
		if id := InstID(s); id != "" {
			return id
		}
	}
	return ""
}

func sevRank(s string) int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	}
	return 2
}

// Sort orders by severity, then subject, then code, then key.
func Sort(fs []*Finding, subjectOrder func(f *Finding) string) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if ra, rb := sevRank(a.Severity), sevRank(b.Severity); ra != rb {
			return ra < rb
		}
		if sa, sb := subjectOrder(a), subjectOrder(b); sa != sb {
			return sa < sb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Key < b.Key
	})
}
