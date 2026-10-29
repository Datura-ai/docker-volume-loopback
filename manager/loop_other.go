//go:build !linux
// +build !linux

package manager

import "github.com/pkg/errors"

func openLoopDeviceIoctl(device string) (loopDevice, error) {
	return nil, errors.Errorf("loop devices exist on Linux only: cannot open '%s'", device)
}
