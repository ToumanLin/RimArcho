//go:build !windows

package fsscan

func storageKind(string) diskKind { return diskUnknown }
