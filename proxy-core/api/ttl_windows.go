//go:build windows

package api

import "syscall"

// setSocketTTL sets IP_TTL on a raw socket fd (Windows: fd is a syscall.Handle).
func setSocketTTL(fd uintptr, ttl int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
}
