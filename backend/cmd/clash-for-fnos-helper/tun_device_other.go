//go:build !linux

package main

import "fmt"

func probeTunDevice() error  { return fmt.Errorf("TUN diagnostics require Linux") }
func createTunDevice() error { return fmt.Errorf("TUN repair requires Linux") }
