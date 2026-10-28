package identity

// QuestionSummary is the small comparison saved with a question, so rows need no
// file-list queries. The detailed comparison remains in the recorded snapshots.
type QuestionSummary struct {
	Available                       bool
	Common, NewFiles, PreviousFiles int
	Differences                     uint8
}

const (
	DifferentName uint8 = 1 << iota
	DifferentPackage
	DifferentAuthor
	DifferentWorkshop
)
