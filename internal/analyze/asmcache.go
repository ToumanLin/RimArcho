package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// AssemblyCache keeps the Harmony facts of DLLs by content (05 §5.3). They depend only on
// a DLL's bytes and its skip flag (harmony.Read reads no other file), so a check takes them
// from the cache when the same DLL was read before, by any Mod and any list. Reading and
// decompiling every DLL was most of a repeated check's CPU time (18 §2). The app keeps it
// in <data>/cache/harmony/.
type AssemblyCache interface {
	// Get returns the facts stored under key, if any.
	Get(key string) (*harmony.Assembly, bool)
	// Put stores the facts of a DLL just read; best effort.
	Put(key string, a *harmony.Assembly)
}

// AsmCacheVersion is part of every key: it changes when harmony.Read's output changes
// without analyze.Version.
const AsmCacheVersion = 1

// readDLL reads a DLL's Harmony facts (tests count the reads).
var readDLL = harmony.Read

// readAssembly returns the Harmony facts of the DLL at path, from c or by reading it.
// Failures are not cached: they are cheap and rare, and read again with the same error.
func readAssembly(c AssemblyCache, path string, skipped bool) (*harmony.Assembly, error) {
	if c == nil {
		return readDLL(path, skipped)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return readDLL(path, skipped) // the same error as without a cache
	}
	sum := sha256.Sum256(b)
	key := AssemblyCacheKey(hex.EncodeToString(sum[:]), skipped)
	if a, ok := c.Get(key); ok {
		return a, nil
	}
	a, err := readDLL(path, skipped)
	if err == nil {
		c.Put(key, a)
	}
	return a, err
}

// AssemblyCacheKey names the facts of a DLL with SHA-256 sum read with the skip flag, for
// this analyzer and cache version.
func AssemblyCacheKey(sum string, skipped bool) string {
	flag := "h"
	if skipped {
		flag = "s"
	}
	return sum + "-" + flag + "-a" + strconv.Itoa(Version) + "c" + strconv.Itoa(AsmCacheVersion)
}
