//go:build race

package store_test

// Race instrumentation changes wall time for SQLite transactions.
const raceBuild = true
