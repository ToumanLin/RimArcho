//go:build !windows

package accounts

import "errors"

func protect([]byte) ([]byte, error)   { return nil, errors.New("账户会话需要 Windows DPAPI") }
func unprotect([]byte) ([]byte, error) { return nil, ErrSession }
func privateDir(string) error          { return ErrSession }
