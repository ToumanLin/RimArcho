package cmds

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// IdentityReasonLines shares the comparison and its wording across CLI/MCP and the
// window. format is fmt.Sprintf for commands, view.Tf for translated interface text.
func IdentityReasonLines(s *store.Store, q *store.Question, format func(string, ...any) string) ([]string, error) {
	e, err := s.QuestionEvidence(q)
	if err == sql.ErrNoRows {
		return []string{format("the comparison records are no longer available")}, nil
	}
	if err != nil {
		return nil, err
	}
	var lines []string
	pair := func(label, a, b string) {
		switch {
		case a == "" || b == "":
			if a == "" {
				a = format("not recorded")
			}
			if b == "" {
				b = format("not recorded")
			}
			lines = append(lines, format("%s: incomplete (%s / %s)", label, a, b))
		case strings.EqualFold(a, b):
			lines = append(lines, format("%s matches: %s", label, a))
		default:
			lines = append(lines, format("%s differs: %s / %s", label, a, b))
		}
	}
	lines = append(lines, format("recorded files and declarations; folders and their Workshop IDs are current; values are new / previous"))
	pair(format("package ID"), e.New.PackageID, e.Candidate.PackageID)
	pair(format("mod name"), e.New.Name, e.Candidate.Name)
	ws := func(v string) string {
		if identity.ValidWorkshopID(v) {
			return v
		}
		return ""
	}
	pair(format("Workshop ID"), ws(e.New.WorkshopID), ws(e.Candidate.WorkshopID))
	pair(format("mod authors"), e.New.Authors, e.Candidate.Authors)
	f := e.Files
	lines = append(lines, format("checked files: %d / %d; %d shared paths, %d identical contents", len(e.New.Files), len(e.Candidate.Files), f.Paths, f.Contents))
	lines = append(lines, format("file differences: %d shared paths have different contents; %d / %d paths occur on only one side", f.Changed, len(e.New.Files)-f.Paths, len(e.Candidate.Files)-f.Paths))
	lines = append(lines, format("mod content in common: %d of %d / %d distinct nonempty files (%d%% overlap)", f.PayloadCommon, f.PayloadA, f.PayloadB, int(f.PayloadSimilarity*100+0.5)))
	lines = append(lines, format("previews, About files, load-folder rules and repeated or empty contents do not count as mod content"))
	for _, p := range f.Examples {
		lines = append(lines, format("content in common: %s", p))
	}
	if f.PayloadCommon > len(f.Examples) {
		lines = append(lines, format("and %d more matching contents", f.PayloadCommon-len(f.Examples)))
	}
	if score := identity.QuestionScore(e.New.PackageID, ws(e.New.WorkshopID), e.New.Files, e.Candidate.PackageID, ws(e.Candidate.WorkshopID), e.Candidate.Files); score >= identity.QuestionSimilarity {
		if identity.SameDecl(e.New.PackageID, ws(e.New.WorkshopID), e.Candidate.PackageID, ws(e.Candidate.WorkshopID)) {
			lines = append(lines, format("asked because the package or Workshop ID and mod content match"))
		} else {
			lines = append(lines, format("asked because at least three distinct mod contents match and overlap is at least 60%%"))
		}
	} else {
		lines = append(lines, format("this old question lacks enough content evidence; the next scan removes it if unanswered"))
	}
	pair(format("current folders"), e.New.Folder, e.Candidate.Folder)
	lines = append(lines, format("folder names and authors alone do not establish a match; textures and audio are not compared by content"))
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
