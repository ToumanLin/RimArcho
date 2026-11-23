//go:build !windows && !linux

package fsscan

func storageKind(string) diskKind { return diskUnknown }
