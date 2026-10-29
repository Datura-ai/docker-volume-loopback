package manager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ashald/docker-volume-loopback/context"
)

const interruptedVolumeName = "interrupted-test"

// interruptedAttach is the state a plugin killed between `losetup --find --show` and `mount`
// leaves: the lease, an empty mount-point dir, and /dev/loop7 attached to the data file with no
// kernel mount. The fake losetup lists the device until the fake loop handle detaches it; the fake
// findmnt knows no mounts.
type interruptedAttach struct {
	manager   Manager
	dataFile  string
	detachErr error
	detaches  int
}

func newInterruptedAttach(t *testing.T, lease string) *interruptedAttach {
	t.Helper()
	root := t.TempDir()
	state := &interruptedAttach{dataFile: filepath.Join(root, "data", interruptedVolumeName), manager: Manager{
		dataDir:  filepath.Join(root, "data"),
		stateDir: filepath.Join(root, "state"),
		mountDir: filepath.Join(root, "mounts"),
	}}
	attached := filepath.Join(root, "attached")
	for _, dir := range []string{state.manager.dataDir, filepath.Join(state.manager.stateDir, interruptedVolumeName), filepath.Join(state.manager.mountDir, interruptedVolumeName)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		state.dataFile: "",
		attached:       "",
		filepath.Join(state.manager.stateDir, interruptedVolumeName, lease): "",
		filepath.Join(root, "losetup"):                                      "#!/bin/sh\n[ -e " + attached + " ] && printf '/dev/loop7\\n'; exit 0\n",
		filepath.Join(root, "findmnt"):                                      "#!/bin/sh\nexit 1\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root)
	backing, err := fileIdentityOf(state.dataFile)
	if err != nil {
		t.Fatal(err)
	}
	openLoopDevice = func(string) (loopDevice, error) {
		if state.detachErr != nil {
			return nil, state.detachErr
		}
		state.detaches++
		_ = os.Remove(attached)
		return fakeLoopDevice{backing: backing, detached: new(bool), closed: new(bool)}, nil
	}
	t.Cleanup(func() { openLoopDevice = openLoopDeviceIoctl })
	return state
}

func (s *interruptedAttach) loopAttached(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(filepath.Dir(s.manager.dataDir), "attached"))
	return err == nil
}

func TestDeleteKeepsTheImageUntilItsLoopDeviceIsDetached(t *testing.T) {
	// an unlinked image keeps its orphan loop alive, and no later Delete can find it by this path
	state := newInterruptedAttach(t, "container-one")
	if err := os.RemoveAll(filepath.Join(state.manager.stateDir, interruptedVolumeName)); err != nil {
		t.Fatal(err)
	}
	state.detachErr = errors.New("cannot open /dev/loop7")

	err := state.manager.Delete(context.New(), interruptedVolumeName)

	if err == nil {
		t.Fatal("Delete succeeded although the loop device could not be detached")
	}
	if _, statErr := os.Stat(state.dataFile); statErr != nil {
		t.Fatalf("Delete removed the image while its loop device is attached: %v", statErr)
	}
	state.detachErr = nil

	if err := state.manager.Delete(context.New(), interruptedVolumeName); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, statErr := os.Stat(state.dataFile); !os.IsNotExist(statErr) {
		t.Fatalf("retry left the image: %v", statErr)
	}
	if state.loopAttached(t) || state.detaches != 1 {
		t.Fatalf("retry left the loop device attached (detaches=%d)", state.detaches)
	}
}

func TestUnMountReclaimsAnInterruptedAttachAndRetriesAsANoOp(t *testing.T) {
	state := newInterruptedAttach(t, "container-one")
	mountPoint := filepath.Join(state.manager.mountDir, interruptedVolumeName)

	if err := state.manager.UnMount(context.New(), interruptedVolumeName, "container-one"); err != nil {
		t.Fatalf("UnMount after the interrupted attach failed: %v", err)
	}

	if state.loopAttached(t) || state.detaches != 1 {
		t.Fatalf("the orphan loop device is still attached (detaches=%d)", state.detaches)
	}
	if _, err := os.Stat(mountPoint); !os.IsNotExist(err) {
		t.Fatalf("the empty mount-point dir was left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.manager.stateDir, interruptedVolumeName)); !os.IsNotExist(err) {
		t.Fatalf("the state dir was left: %v", err)
	}

	// Docker retries the Unmount it saw fail: no lease, no loop, no mount dir -> nothing to do
	if err := state.manager.UnMount(context.New(), interruptedVolumeName, "container-one"); err != nil {
		t.Fatalf("UnMount retry failed: %v", err)
	}
	if state.detaches != 1 {
		t.Fatalf("the retry detached again (detaches=%d)", state.detaches)
	}
}

func TestUnMountKeepsAnUnmountedMountDirThatHoldsFiles(t *testing.T) {
	// never RemoveAll on a dir that is not a kernel mount: files there are not this volume's
	state := newInterruptedAttach(t, "container-one")
	mountPoint := filepath.Join(state.manager.mountDir, interruptedVolumeName)
	if err := os.WriteFile(filepath.Join(mountPoint, "not-ours"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	err := state.manager.UnMount(context.New(), interruptedVolumeName, "container-one")

	if err == nil {
		t.Fatal("UnMount removed a mount-point dir that holds files")
	}
	if _, statErr := os.Stat(filepath.Join(mountPoint, "not-ours")); statErr != nil {
		t.Fatalf("the foreign file is gone: %v", statErr)
	}
	if state.detaches != 1 {
		t.Fatalf("the orphan loop was not detached first (detaches=%d)", state.detaches)
	}
}
