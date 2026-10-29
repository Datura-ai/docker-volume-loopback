package manager

import (
	"github.com/ashald/docker-volume-loopback/context"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

func validateName(ctx *context.Context, name string) (err error) {
	ctx = ctx.
		Field(":func", "manager/validateName")

	ctx.
		Level(context.Debug).
		Field(":param/name", name).
		Message("invoked")

	defer func() {
		if err != nil {
			ctx.
				Level(context.Error).
				Field(":return/err", err).
				Message("failed with an error")
			return
		} else {
			ctx.
				Level(context.Debug).
				Message("finished")
		}
	}()

	if name == "" {
		err = errors.New("invalid volume name: cannot be an empty string")
		return
	}

	if !NameRegex.MatchString(name) {
		err = errors.Errorf("invalid volume name - '%s' does not match allowed pattern '%s'", name, NamePattern)
		return
	}

	return
}

func runCommand(ctx *context.Context, name string, args ...string) (output string, err error) {
	ctx = ctx.
		Field(":func", "manager/runCommand")

	ctx.
		Level(context.Debug).
		Field(":param/name", name).
		Field(":param/args", args).
		Message("invoked")

	defer func() {
		if err != nil {
			ctx.
				Level(context.Error).
				Field(":return/err", err).
				Message("failed with an error")
			return
		} else {
			ctx.
				Level(context.Debug).
				Field(":return/output", output).
				Message("finished")
		}
	}()

	outBytes, err := exec.Command(name, args...).CombinedOutput()
	output = strings.TrimSpace(string(outBytes[:]))

	return
}

var loopDevicePattern = regexp.MustCompile(`^/dev/loop[0-9]+$`)

// runCommandStdout is runCommand for a command whose stdout is consumed as a value: stderr is kept
// out of the result and only quoted in the error. losetup prints its warnings on stderr while still
// succeeding, and with CombinedOutput they landed in front of the device path.
func runCommandStdout(ctx *context.Context, name string, args ...string) (output string, err error) {
	ctx = ctx.
		Field(":func", "manager/runCommandStdout")

	ctx.
		Level(context.Debug).
		Field(":param/name", name).
		Field(":param/args", args).
		Message("invoked")

	defer func() {
		if err != nil {
			ctx.
				Level(context.Error).
				Field(":return/err", err).
				Message("failed with an error")
			return
		} else {
			ctx.
				Level(context.Debug).
				Field(":return/output", output).
				Message("finished")
		}
	}()

	outBytes, err := exec.Command(name, args...).Output()
	output = strings.TrimSpace(string(outBytes))
	if exitErr, ok := err.(*exec.ExitError); ok {
		err = errors.Wrapf(err, "%s", strings.TrimSpace(string(exitErr.Stderr)))
	}

	return
}

// loopDeviceFromLosetup takes the device path out of `losetup --find --show` stdout: the last
// non-empty line, which must be a plain /dev/loopN, so that a later `losetup -d` and `mount` never
// receive anything else.
func loopDeviceFromLosetup(output string) (string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	device := strings.TrimSpace(lines[len(lines)-1])
	if !loopDevicePattern.MatchString(device) {
		return "", errors.Errorf("losetup did not print a loop device: %q", output)
	}
	return device, nil
}

func isMountPoint(path string) (bool, error) {
	return findmntHasEntry("--mountpoint", path)
}

func isLoopDeviceMounted(device string) (bool, error) {
	return findmntHasEntry("--source", device)
}

// findmntHasEntry reports whether the kernel mount table has an entry for the selector (a mount
// point or a source device); findmnt's exit 1 with nothing printed is "no such mount".
func findmntHasEntry(selector string, value string) (bool, error) {
	data, err := exec.Command("findmnt", "--noheadings", "--raw", selector, value, "--output", "TARGET").Output()
	output := strings.TrimSpace(string(data))
	if err == nil {
		if output == "" {
			return false, errors.Errorf("findmnt returned no mount information for '%s'", value)
		}
		return true, nil
	}
	detail := ""
	if exit, ok := err.(*exec.ExitError); ok {
		detail = strings.TrimSpace(string(exit.Stderr))
		if exit.ExitCode() == 1 && output == "" && detail == "" {
			return false, nil
		}
	}
	return false, errors.Wrapf(err, "cannot check kernel mount state for '%s': %s", value, detail)
}

// fileIdentity is what the kernel records for a loop device's backing file: its filesystem and inode.
type fileIdentity struct {
	Device uint64
	Inode  uint64
}

func fileIdentityOf(path string) (fileIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, errors.Errorf("cannot stat '%s'", path)
	}
	return fileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}, nil
}

// loopDevice is an open loop device: its backing file is read and the device detached through the
// one descriptor, so the loop number cannot be freed and given to another image in between.
type loopDevice interface {
	BackingFile() (fileIdentity, error)
	Detach() error
	Close() error
}

var errLoopDeviceFree = errors.New("the loop device has no backing file")

// replaced in tests: there is no loop device to open on a developer machine
var openLoopDevice = openLoopDeviceIoctl

// detachLoopIfBackedBy detaches device only if it is still backed by the given file: `losetup -d`
// trusts the device name, and between a listing and the detach autoclear can free the number and
// another process attach its own image to it.
func detachLoopIfBackedBy(device string, backing fileIdentity) (bool, error) {
	handle, err := openLoopDevice(device)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "cannot open the loop device '%s'", device)
	}
	defer handle.Close()
	attached, err := handle.BackingFile()
	if err == errLoopDeviceFree {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrapf(err, "cannot read the backing file of '%s'", device)
	}
	if attached != backing {
		return false, nil
	}
	if err := handle.Detach(); err != nil {
		return false, errors.Wrapf(err, "cannot detach the stray loop device '%s'", device)
	}
	return true, nil
}

// detachStrayLoops detaches the loop devices of dataFilePath that back no mount: a plugin killed
// between `losetup --find --show` and `mount` leaves one attached without autoclear, and nothing
// else frees it (UnMount has no mount to `umount -d`, Delete removes the file only). Mounted
// devices stay, whoever mounted them.
func detachStrayLoops(ctx *context.Context, dataFilePath string) error {
	backing, err := fileIdentityOf(dataFilePath)
	if err != nil {
		return errors.Wrapf(err, "cannot identify the data file '%s'", dataFilePath)
	}
	devices, err := runCommandStdout(ctx.Derived(), "losetup", "--noheadings", "--output", "NAME",
		"--associated", dataFilePath)
	if err != nil {
		return errors.Wrapf(err, "cannot list the loop devices of '%s'", dataFilePath)
	}
	for _, device := range strings.Fields(devices) {
		if !loopDevicePattern.MatchString(device) {
			return errors.Errorf("losetup listed something that is not a loop device: %q", device)
		}
		mounted, err := isLoopDeviceMounted(device)
		if err != nil {
			return err
		}
		if mounted {
			continue
		}
		detached, err := detachLoopIfBackedBy(device, backing)
		if err != nil {
			return err
		}
		ctx.
			Level(context.Debug).
			Field("loop-device", device).
			Field("detached", detached).
			Message("checked a loop device of the data file that backs no mount")
	}
	return nil
}

// mountBelongsToDataFile reports whether the filesystem mounted at mountPoint sits on a loop
// device backed by dataFilePath (a same-name volume of another plugin instance, or a foreign
// mount left at the path, must not be handed out or unmounted as this volume).
func mountBelongsToDataFile(mountPoint string, dataFilePath string) (bool, error) {
	// stdout only: a findmnt warning on stderr would be compared as part of the source path
	source, err := exec.Command("findmnt", "--noheadings", "--raw", "--first-only",
		"--mountpoint", mountPoint, "--output", "SOURCE").Output()
	if err != nil {
		detail := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			detail = string(exitErr.Stderr)
		}
		return false, errors.Wrapf(err, "cannot read the mount source at '%s': %s", mountPoint, detail)
	}
	devices, err := exec.Command("losetup", "--noheadings", "--output", "NAME",
		"--associated", dataFilePath).CombinedOutput()
	if err != nil {
		return false, errors.Wrapf(err, "cannot list the loop devices of '%s': %s", dataFilePath, devices)
	}
	for _, device := range strings.Fields(string(devices)) {
		if device == strings.TrimSpace(string(source)) {
			return true, nil
		}
	}
	return false, nil
}
