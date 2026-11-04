//go:build !windows

package netcfg

// Elsewhere the environment's proxy variables are the system proxy.
func readSystemProxy() systemProxySettings { return systemProxySettings{} }
