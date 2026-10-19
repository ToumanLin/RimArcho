package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/source"
)

// Mod is an Instance joined with its source and current snapshot, for queries and output.
type Mod struct {
	// Player data of the lineage (04 §7.1), filled by the app's index.
	Alias, Note string
	Tags        []string

	ID           string
	LineageID    string
	LineageBasis string
	SourceID     string
	SourceLabel  string
	Kind         source.Kind
	Rel          string
	RealPath     string
	Status       string
	Residue      bool
	Unloadable   string // "about-disabled" etc.
	FirstSeen    time.Time
	LastSeen     time.Time
	TombstoneAt  time.Time

	SnapID            int64
	SnapHash          []byte
	PackageID         string
	PackageIDRaw      string
	Name              string
	Authors           string
	DeclaredWorkshop  string
	SupportedVersions string
	DeclBlob          string
	ACFManifest       string
	ACFTimeUpdated    int64
	ACFSize           int64
	GitHead           string
	Origin            string
	FileCount         int
	TotalSize         int64
	SemanticCount     int

	store *Store // for loading DeclBlob on demand (lite queries)
}

// WorkshopID is the folder name under workshop-like sources, else the declared ID.
func (m *Mod) WorkshopID() string {
	if m.Kind.WorkshopLike() {
		return m.Rel
	}
	return m.DeclaredWorkshop
}

// DisplayName falls back to the folder name when About.xml has no <name>.
func (m *Mod) DisplayName() string {
	if m.Alias != "" {
		return m.Alias // the player's name for it comes first (04 §7.1)
	}
	if m.Name != "" {
		return m.Name
	}
	return m.Rel
}

// Decl decodes the stored declaration, loading it first if the Mod came from a lite query.
func (m *Mod) Decl() scan.Decl {
	if m.DeclBlob == "" && m.store != nil && m.SnapID != 0 {
		m.store.DB.QueryRow(`SELECT decl_blob FROM snapshot WHERE id=?`, m.SnapID).Scan(&m.DeclBlob)
	}
	var d scan.Decl
	json.Unmarshal([]byte(m.DeclBlob), &d)
	return d
}

const modCols = `i.id, i.lineage_id, i.lineage_basis, i.source_id, s.label, s.kind, i.rel_path, i.real_path, i.status, i.residue, i.unloadable,
	i.first_seen, i.last_seen, coalesce(i.tombstone_at, 0),
	coalesce(n.id, 0), coalesce(n.snapshot_hash, x''), coalesce(n.package_id, ''), coalesce(n.package_id_raw, ''), coalesce(n.name, ''),
	coalesce(n.authors, ''), coalesce(n.workshop_id_declared, ''), coalesce(n.supported_versions, ''), %s,
	coalesce(n.acf_manifest, ''), coalesce(n.acf_time_updated, 0), coalesce(n.acf_size, 0), coalesce(n.git_head, ''), coalesce(n.origin, ''),
	coalesce(n.file_count, 0), coalesce(n.total_size, 0), coalesce(n.semantic_count, 0)`

const modFrom = ` FROM instance i JOIN source s ON s.id = i.source_id LEFT JOIN snapshot n ON n.id = i.current_snapshot_id `

func scanMod(row interface{ Scan(...any) error }) (*Mod, error) {
	var m Mod
	var kind string
	var first, last, tomb int64
	var residue int
	err := row.Scan(&m.ID, &m.LineageID, &m.LineageBasis, &m.SourceID, &m.SourceLabel, &kind, &m.Rel, &m.RealPath, &m.Status, &residue, &m.Unloadable,
		&first, &last, &tomb, &m.SnapID, &m.SnapHash, &m.PackageID, &m.PackageIDRaw, &m.Name, &m.Authors, &m.DeclaredWorkshop,
		&m.SupportedVersions, &m.DeclBlob, &m.ACFManifest, &m.ACFTimeUpdated, &m.ACFSize, &m.GitHead, &m.Origin,
		&m.FileCount, &m.TotalSize, &m.SemanticCount)
	if err != nil {
		return nil, err
	}
	m.Kind = source.Kind(kind)
	m.Residue = residue != 0
	m.FirstSeen, m.LastSeen = time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC()
	if tomb != 0 {
		m.TombstoneAt = time.Unix(tomb, 0).UTC()
	}
	return &m, nil
}

var (
	modColsFull = fmt.Sprintf(modCols, "coalesce(n.decl_blob, '{}')")
	// modColsLite leaves out decl_blob: listing every instance with its declaration churns
	// SQLite's allocator, which never returns memory, so the TUI only loads it on demand.
	modColsLite = fmt.Sprintf(modCols, "''")
)

// Mods returns every Instance of an environment (live and tombstones). Declarations are
// loaded on demand by Mod.Decl (see LoadDecls for a batch).
func (s *Store) Mods(envID string) ([]*Mod, error) { return s.mods(envID, modColsLite) }

// ModsFull is Mods with declarations included.
func (s *Store) ModsFull(envID string) ([]*Mod, error) { return s.mods(envID, modColsFull) }

func (s *Store) mods(envID, cols string) ([]*Mod, error) {
	rows, err := s.DB.Query(`SELECT `+cols+modFrom+` WHERE i.environment_id=? ORDER BY i.id`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mod
	for rows.Next() {
		m, err := scanMod(rows)
		if err != nil {
			return nil, err
		}
		m.store = s
		out = append(out, m)
	}
	return out, rows.Err()
}

// LoadDecls fills DeclBlob for many instances in a few queries.
func (s *Store) LoadDecls(ms []*Mod) error {
	need := map[int64][]*Mod{}
	var ids []any
	for _, m := range ms {
		if m.DeclBlob == "" && m.SnapID != 0 {
			if need[m.SnapID] == nil {
				ids = append(ids, m.SnapID)
			}
			need[m.SnapID] = append(need[m.SnapID], m)
		}
	}
	for len(ids) > 0 {
		n := min(len(ids), 400)
		rows, err := s.DB.Query(`SELECT id, decl_blob FROM snapshot WHERE id IN (`+placeholders(n)+`)`, ids[:n]...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var blob string
			rows.Scan(&id, &blob)
			for _, m := range need[id] {
				m.DeclBlob = blob
			}
		}
		rows.Close()
		ids = ids[n:]
	}
	return nil
}

// Mod returns one Instance, or nil.
func (s *Store) Mod(id string) (*Mod, error) {
	m, err := scanMod(s.DB.QueryRow(`SELECT `+modColsFull+modFrom+` WHERE i.id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// Alias is an extra location of an Instance (02 §3.3).
type Alias struct {
	SourceID, SourceLabel, Rel, RealPath string
}

// Aliases lists an Instance's alias locations.
func (s *Store) Aliases(instanceID string) ([]Alias, error) {
	rows, err := s.DB.Query(`SELECT a.source_id, s.label, a.rel_path, a.real_path FROM instance_alias a JOIN source s ON s.id=a.source_id
		WHERE a.instance_id=? ORDER BY s.label, a.rel_path`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.SourceID, &a.SourceLabel, &a.Rel, &a.RealPath); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AliasCount counts alias rows per instance in an environment.
func (s *Store) AliasCount(envID string) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM instance_alias a JOIN instance i ON i.id=a.instance_id WHERE i.environment_id=?`, envID).Scan(&n)
	return n, err
}

// Known loads every Instance of an environment in the shape reconciliation needs.
func (s *Store) Known(envID string) ([]*identity.Known, error) {
	mods, err := s.ModsFull(envID)
	if err != nil {
		return nil, err
	}
	out := make([]*identity.Known, 0, len(mods))
	for _, m := range mods {
		k := &identity.Known{ID: m.ID, LineageID: m.LineageID, SourceID: m.SourceID, Kind: m.Kind, Label: m.SourceLabel, Rel: m.Rel, Status: m.Status}
		if m.SnapID != 0 {
			sn := &identity.Snap{ID: m.SnapID, Decl: m.Decl(), WorkshopID: m.WorkshopID(), ACFManifest: m.ACFManifest, GitHead: m.GitHead}
			copy(sn.Hash[:], m.SnapHash)
			if m.Origin != "" {
				var og scan.Origin
				if json.Unmarshal([]byte(m.Origin), &og) == nil {
					sn.Origin = &og
				}
			}
			k.Snap = sn
		}
		out = append(out, k)
	}
	return out, nil
}

// LineageIDs returns the set of lineage IDs of an environment.
func (s *Store) LineageIDs(envID string) (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT id FROM lineage WHERE environment_id=?`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out[id] = true
	}
	return out, rows.Err()
}

// SemanticSet loads the semantic layer of a snapshot.
func (s *Store) SemanticSet(snapshotID int64) (scan.SemanticSet, error) {
	rows, err := s.DB.Query(`SELECT lower(rel_path), hash FROM snapshot_file WHERE snapshot_id=? AND hash IS NOT NULL`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := scan.SemanticSet{}
	for rows.Next() {
		var p string
		var h []byte
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		var hh scan.Hash
		copy(hh[:], h)
		out[p] = hh
	}
	return out, rows.Err()
}

// CurrentHashes returns the current snapshot hash of every Instance, keyed by location.
func (s *Store) CurrentHashes(envID string) (map[string]scan.Hash, error) {
	rows, err := s.DB.Query(`SELECT i.source_id, i.rel_path, n.snapshot_hash FROM instance i
		JOIN snapshot n ON n.id = i.current_snapshot_id WHERE i.environment_id=?`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]scan.Hash{}
	for rows.Next() {
		var src, rel string
		var h []byte
		if err := rows.Scan(&src, &rel, &h); err != nil {
			return nil, err
		}
		var hh scan.Hash
		copy(hh[:], h)
		out[scan.LocKey(src, rel)] = hh
	}
	return out, rows.Err()
}

// CurrentFiles loads the current semantic files (the only ones with hashes to reuse) of all
// Instances of an environment, keyed by location (scan.LocKey) and lower-cased file path.
func (s *Store) CurrentFiles(envID string) (map[string]map[string]scan.File, error) {
	rows, err := s.DB.Query(`SELECT i.source_id, i.rel_path, f.rel_path, f.size, f.mtime, f.hash FROM instance i
		JOIN snapshot_file f ON f.snapshot_id = i.current_snapshot_id WHERE i.environment_id=? AND f.hash IS NOT NULL`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]scan.File{}
	for rows.Next() {
		var src, irel string
		var f scan.File
		var h []byte
		if err := rows.Scan(&src, &irel, &f.Rel, &f.Size, &f.MTime, &h); err != nil {
			return nil, err
		}
		if h != nil {
			copy(f.Hash[:], h)
			f.HasHash = true
		}
		k := scan.LocKey(src, irel)
		m := out[k]
		if m == nil {
			m = map[string]scan.File{}
			out[k] = m
		}
		m[strings.ToLower(f.Rel)] = f
	}
	return out, rows.Err()
}
