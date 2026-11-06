package steamworks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PublishRequest is the JSON file passed to `steam-worker publish <absolute file>`.
// It lives outside Content, so it cannot become part of the Workshop item.
// Visibility nil leaves Steam's current/default visibility unchanged.
type PublishRequest struct {
	ID          uint64  `json:"id"`
	Content     string  `json:"content"`
	Preview     string  `json:"preview,omitempty"`
	Version     string  `json:"version"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Note        string  `json:"note"`
	Visibility  *uint32 `json:"visibility,omitempty"`
	IDFile      string  `json:"id_file"` // source About/PublishedFileId.txt; saved immediately after CreateItem
}

// ReadPublishRequest validates local inputs before loading or calling Steam.
func ReadPublishRequest(path string) (PublishRequest, error) {
	var r PublishRequest
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return r, err
	}
	if fi.Size() > 64<<10 {
		return r, errors.New("publish request exceeds 64 KB")
	}
	if err := json.NewDecoder(f).Decode(&r); err != nil {
		return r, err
	}
	if !filepath.IsAbs(r.Content) || !filepath.IsAbs(r.IDFile) || r.Preview != "" && !filepath.IsAbs(r.Preview) {
		return r, errors.New("publish content, preview and ID file paths must be absolute")
	}
	if r.Title == "" || r.Version == "" || strings.ContainsAny(r.Version, ",\r\n") {
		return r, errors.New("publish title and version are required; version must be one tag")
	}
	for _, s := range []string{r.Content, r.Preview, r.Version, r.Title, r.Description, r.Note, r.IDFile} {
		if strings.ContainsRune(s, 0) {
			return r, errors.New("publish request contains a NUL character")
		}
	}
	if r.Visibility != nil && *r.Visibility > 3 {
		return r, errors.New("invalid Workshop visibility")
	}
	return r, nil
}

// PublishResultMessage explains the account restrictions that can reject an upload.
func PublishResultMessage(result int32) string {
	switch result {
	case 15:
		return "Steam denied access; check that this account owns RimWorld and owns the Workshop item"
	case 24:
		return "Steam denied permission; check this account's community restrictions"
	case 17:
		return "this Steam account is banned from publishing Workshop content"
	case 44:
		return "the Steam Workshop is read-only for this account; a recent password or email change can temporarily block uploads"
	case 112:
		return "this Steam account is limited and cannot publish Workshop content"
	default:
		return fmt.Sprintf("Steam rejected the Workshop upload (result %d)", result)
	}
}

// The publish operation emits these lines in addition to ERROR:
// CREATED <id> <legal-required>, UPLOAD <status> <processed> <total>,
// PUBLISHED <id> <legal-required>, DONE publish 1. Legal flags are 0 or 1.
// CREATED is emitted even if saving the new ID or submitting later fails.
const Publish = "publish"
