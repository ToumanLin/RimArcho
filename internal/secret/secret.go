// Package secret keeps small secrets (the Archo Agent's model API key, 15 §3.1) in Windows Credential
// Manager, as generic credentials of the current user. Nothing is written to RimArcho's own files.
// Other systems have no store yet: Get finds nothing and Set returns ErrUnsupported.
package secret

import "errors"

// ErrUnsupported is returned where the system has no credential store RimArcho uses.
var ErrUnsupported = errors.New("no credential store on this system")

// ErrNotFound is returned by Get when no secret is stored under the name.
var ErrNotFound = errors.New("secret not found")

// Prefix starts every credential name RimArcho writes ("RimArcho/agent-api-key").
const Prefix = "RimArcho/"

// Set stores value under name (Prefix is added), replacing any earlier value.
func Set(name, value string) error { return set(Prefix+name, value) }

// Get returns the value stored under name, or ErrNotFound.
func Get(name string) (string, error) { return get(Prefix + name) }

// Delete removes the value stored under name; a missing value is not an error.
func Delete(name string) error { return del(Prefix + name) }
