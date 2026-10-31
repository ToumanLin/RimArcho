package selfupdate

import (
	"cmp"
	"regexp"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
var numberedIdentifier = regexp.MustCompile(`^([A-Za-z]+)([0-9]+)$`)

type version struct {
	core []string
	pre  []string
}

func parseVersion(s string) (version, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	v := version{core: m[1:4]}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
		for _, id := range v.pre {
			if numeric(id) && len(id) > 1 && id[0] == '0' {
				return version{}, false
			}
		}
	}
	return v, true
}

// Compare orders semantic versions, comparing rc2-style suffixes numerically.
func Compare(a, b string) (int, bool) {
	x, xok := parseVersion(a)
	y, yok := parseVersion(b)
	if !xok || !yok {
		return 0, false
	}
	for i := range x.core {
		if n := compareNumber(x.core[i], y.core[i]); n != 0 {
			return n, true
		}
	}
	if len(x.pre) == 0 || len(y.pre) == 0 {
		return cmp.Compare(len(y.pre), len(x.pre)), true
	}
	for i := 0; i < min(len(x.pre), len(y.pre)); i++ {
		if n := compareIdentifier(x.pre[i], y.pre[i]); n != 0 {
			return n, true
		}
	}
	return cmp.Compare(len(x.pre), len(y.pre)), true
}

// Newer reports whether latest sorts after current; invalid versions return false.
func Newer(current, latest string) bool {
	n, ok := Compare(current, latest)
	return ok && n < 0
}

func numeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// Decimal strings avoid integer overflow on arbitrarily long version components.
func compareNumber(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if n := cmp.Compare(len(a), len(b)); n != 0 {
		return n
	}
	return strings.Compare(a, b)
}

func compareIdentifier(a, b string) int {
	an, bn := numeric(a), numeric(b)
	if an && bn {
		return compareNumber(a, b)
	}
	if an != bn {
		if an {
			return -1
		}
		return 1
	}
	x, y := numberedIdentifier.FindStringSubmatch(a), numberedIdentifier.FindStringSubmatch(b)
	if x != nil && y != nil {
		if n := strings.Compare(x[1], y[1]); n != 0 {
			return n
		}
		return compareNumber(x[2], y[2])
	}
	return strings.Compare(a, b)
}
