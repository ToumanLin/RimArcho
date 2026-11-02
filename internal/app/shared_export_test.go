package app

import "github.com/ToumanLin/RimArcho/internal/loadout"

// Test hooks for shared_test.go.
func SameLinks(a, b *App) bool                                  { return a.linkInst != nil && a.linkInst == b.linkInst }
func ForgetLinksOf(a *App)                                      { forgetLinks(a.Store) }
func SharedRegistryFor(c []loadout.Candidate) *loadout.Registry { return sharedRegistry(c) }
