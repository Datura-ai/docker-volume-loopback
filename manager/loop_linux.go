//go:build linux
// +build linux

package manager

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
)

// linux/loop.h; the vendored x/sys predates its loop ioctl helpers and is not a direct dependency
const (
	loopClrFd       = 0x4C01
	loopGetStatus64 = 0x4C05
)

// struct loop_info64
type loopInfo64 struct {
	Device         uint64
	Inode          uint64
	Rdevice        uint64
	Offset         uint64
	Sizelimit      uint64
	Number         uint32
	EncryptType    uint32
	EncryptKeySize uint32
	Flags          uint32
	FileName       [64]byte
	CryptName      [64]byte
	EncryptKey     [32]byte
	Init           [2]uint64
}

type ioctlLoopDevice struct {
	file *os.File
}

func openLoopDeviceIoctl(device string) (loopDevice, error) {
	file, err := os.OpenFile(device, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return ioctlLoopDevice{file: file}, nil
}

func (d ioctlLoopDevice) BackingFile() (fileIdentity, error) {
	var info loopInfo64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.file.Fd(), loopGetStatus64, uintptr(unsafe.Pointer(&info)))
	if errno == syscall.ENXIO {
		return fileIdentity{}, errLoopDeviceFree
	}
	if errno != 0 {
		return fileIdentity{}, errors.Wrapf(errno, "LOOP_GET_STATUS64 on '%s'", d.file.Name())
	}
	return fileIdentity{Device: info.Device, Inode: info.Inode}, nil
}

func (d ioctlLoopDevice) Detach() error {
	// on a device still open elsewhere the kernel only marks it autoclear, as `losetup -d` does
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.file.Fd(), loopClrFd, 0); errno != 0 {
		return errors.Wrapf(errno, "LOOP_CLR_FD on '%s'", d.file.Name())
	}
	return nil
}

func (d ioctlLoopDevice) Close() error {
	return d.file.Close()
}
