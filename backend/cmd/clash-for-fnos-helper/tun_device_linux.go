//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func probeTunDevice() error {
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFCHR || stat.Rdev != 10<<8|200 {
		return fmt.Errorf("unexpected TUN device number")
	}
	var features uint32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x800454cf, uintptr(unsafe.Pointer(&features))) // TUNGETFEATURES
	if errno != 0 {
		return errno
	}
	return nil
}
func createTunDevice() error {
	// Never replace an existing file, link, or device; verify the parent too.
	if err := os.Mkdir("/dev/net", 0755); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat("/dev/net")
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("/dev/net is not a directory")
	}
	return syscall.Mknod("/dev/net/tun", syscall.S_IFCHR|0600, 10<<8|200)
}
