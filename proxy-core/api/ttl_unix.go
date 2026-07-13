//go:build !windows

package api

import "syscall"

// setSocketTTL sets IP_TTL on a raw socket fd (Unix: fd is an int).
func setSocketTTL(fd uintptr, ttl int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
}
