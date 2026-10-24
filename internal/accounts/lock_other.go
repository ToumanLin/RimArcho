//go:build !windows

package accounts

func platformLock(string) func() { return func() {} }
