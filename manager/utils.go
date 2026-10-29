package manager

import (
	"github.com/ashald/docker-volume-loopback/context"
	"github.com/pkg/errors"
	"os/exec"
	"regexp"
	"strings"
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
	data, err := exec.Command("findmnt", "--noheadings", "--raw", "--mountpoint", path, "--output", "TARGET").CombinedOutput()
	output := strings.TrimSpace(string(data))
	if err == nil {
		if output == "" {
			return false, errors.Errorf("findmnt returned no mount information for '%s'", path)
		}
		return true, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && output == "" {
		return false, nil
	}
	return false, errors.Wrapf(err, "cannot check kernel mount state at '%s': %s", path, output)
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
