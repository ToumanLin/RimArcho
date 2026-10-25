//go:build !windows

package accounts

import "errors"

func protect([]byte) ([]byte, error) {
	return nil, errors.New("account sessions require Windows DPAPI")
}
func unprotect([]byte) ([]byte, error) { return nil, ErrSession }
func privateDir(string) error          { return ErrSession }
