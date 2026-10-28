package cmds

import (
	"database/sql"
	"fmt"
	"path"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// IdentityHeadline uses only the compact summary stored with a question.
func IdentityHeadline(v identity.QuestionSummary, f func(string, ...any) string) string {
	if !v.Available {
		return f("the comparison records are no longer available")
	}
	if v.Common == 0 {
		return f("not enough matching content to support this question")
	}
	var headline string
	if v.Common == v.NewFiles && v.Common == v.PreviousFiles {
		headline = f("%d content files are identical", v.Common)
	} else {
		union := v.NewFiles + v.PreviousFiles - v.Common
		headline = f("%d content files match (%d%% overlap)", v.Common, (100*v.Common+union/2)/union)
	}
	var different []string
	for _, item := range []struct {
		bit   uint8
		label string
	}{
		{identity.DifferentName, "name"}, {identity.DifferentPackage, "package ID"},
		{identity.DifferentAuthor, "mod authors"}, {identity.DifferentWorkshop, "Workshop ID"},
	} {
		if v.Differences&item.bit != 0 {
			different = append(different, f(item.label))
		}
	}
	if len(different) > 0 {
		names := different[0]
		for _, label := range different[1:] {
			names = f("%s, %s", names, label)
		}
		headline = f("%s; differences: %s", headline, names)
	}
	return headline
}

type IdentityComparisonRow struct {
	Label        string `json:"label"`
	New          string `json:"new"`
	Previous     string `json:"previous"`
	Match        bool   `json:"match"`
	NewPath      string `json:"newPath,omitempty"`
	PreviousPath string `json:"previousPath,omitempty"`
}

type IdentityComparisonDoc struct {
	Headline string                  `json:"headline"`
	Rows     []IdentityComparisonRow `json:"rows"`
	Files    []string                `json:"files"`
	More     int                     `json:"more"`
}

// CompareIdentity loads the detailed comparison only when requested. Both-missing
// values and method explanations add nothing to the player's decision.
func CompareIdentity(s *store.Store, q *store.Question, f func(string, ...any) string) (*IdentityComparisonDoc, error) {
	out := &IdentityComparisonDoc{Headline: IdentityHeadline(q.ComparisonSummary(), f)}
	e, err := s.QuestionEvidence(q)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Headline = IdentityHeadline(store.SummaryOf(e), f)
	pair := func(label, a, b, ap, bp string) {
		if a == "" && b == "" {
			return
		}
		row := IdentityComparisonRow{Label: f(label), New: a, Previous: b, Match: strings.EqualFold(a, b), NewPath: ap, PreviousPath: bp}
		if row.New == "" {
			row.New = f("not recorded")
		}
		if row.Previous == "" {
			row.Previous = f("not recorded")
		}
		out.Rows = append(out.Rows, row)
	}
	ws := func(v string) string {
		if identity.ValidWorkshopID(v) {
			return v
		}
		return ""
	}
	pair("name", e.New.Name, e.Candidate.Name, "", "")
	pair("package ID", e.New.PackageID, e.Candidate.PackageID, "", "")
	pair("author", e.New.Authors, e.Candidate.Authors, "", "")
	pair("Workshop ID", ws(e.New.WorkshopID), ws(e.Candidate.WorkshopID), "", "")
	folder := func(v string) string {
		if v == "" {
			return ""
		}
		return path.Base(strings.TrimRight(strings.ReplaceAll(v, `\`, "/"), "/"))
	}
	pair("comparison folder", folder(e.New.Folder), folder(e.Candidate.Folder), e.New.Folder, e.Candidate.Folder)
	// Folder names may match in different locations.
	if len(out.Rows) > 0 {
		row := &out.Rows[len(out.Rows)-1]
		if row.NewPath != "" || row.PreviousPath != "" {
			row.Match = strings.EqualFold(row.NewPath, row.PreviousPath)
		}
	}
	out.Files = e.Files.Examples[:min(5, len(e.Files.Examples))]
	out.More = e.Files.PayloadCommon - len(out.Files)
	return out, nil
}

func IdentityReasonLines(s *store.Store, q *store.Question, f func(string, ...any) string) ([]string, error) {
	v, err := CompareIdentity(s, q, f)
	if err != nil {
		return nil, err
	}
	lines := []string{v.Headline}
	for _, row := range v.Rows {
		if row.Match {
			lines = append(lines, f("%s matches: %s", row.Label, row.New))
		} else {
			lines = append(lines, f("%s: %s / %s", row.Label, row.New, row.Previous))
		}
	}
	for _, file := range v.Files {
		lines = append(lines, f("matching file: %s", file))
	}
	if v.More > 0 {
		lines = append(lines, f("and %d more files", v.More))
	}
	return lines, nil
}

func addIdentityReasons(s *store.Store, q *store.Question, add func(string)) error {
	lines, err := IdentityReasonLines(s, q, fmt.Sprintf)
	if err != nil {
		return err
	}
	for _, line := range lines {
		add(line)
	}
	return nil
}
