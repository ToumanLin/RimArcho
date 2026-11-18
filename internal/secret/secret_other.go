//go:build !windows

package secret

func set(string, string) error   { return ErrUnsupported }
func get(string) (string, error) { return "", ErrNotFound }
func del(string) error           { return nil }
