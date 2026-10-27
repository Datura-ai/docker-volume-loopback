//go:build linux
// +build linux

package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashald/docker-volume-loopback/context"
)

const recoveryVolumeName = "recovery-test"

func recoveryManager(t *testing.T, fs string) Manager {
	t.Helper()
	return recoveryManagerWithSize(t, fs, 1<<30)
}

func recoveryManagerWithSize(t *testing.T, fs string, sizeInBytes int64) Manager {
	t.Helper()
	if os.Getenv("LOOPBACK_MOUNT_TESTS") != "1" {
		t.Skip("set LOOPBACK_MOUNT_TESTS=1 inside a disposable privileged Linux container")
	}
	root := t.TempDir()
	m := Manager{
		dataDir:  filepath.Join(root, "data"),
		stateDir: filepath.Join(root, "state"),
		mountDir: filepath.Join(root, "mounts"),
	}
	if err := os.Mkdir(m.mountDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Only the mount created under this test's private temporary directory.
		exec.Command("umount", filepath.Join(m.mountDir, recoveryVolumeName)).Run()
	})
	if err := m.Create(context.New(), recoveryVolumeName, sizeInBytes, true, fs, -1, -1, 0); err != nil {
		t.Fatal(err)
	}
	return m
}

func mountRecoveryVolume(t *testing.T, m Manager, lease string) string {
	t.Helper()
	path, err := m.Mount(context.New(), recoveryVolumeName, lease)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(m.mountDir, recoveryVolumeName)
	if path != want {
		t.Fatalf("Mount returned %q without an error, want %q", path, want)
	}
	return path
}

func writeRecoveryMarker(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(filepath.Join(path, "marker"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString("persisted before reconnect\n"); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveryMarker(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, "marker"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "persisted before reconnect\n" {
		t.Fatalf("unexpected recovered marker: %q", data)
	}
}

func disconnectRecoveryVolume(t *testing.T, path string) {
	t.Helper()
	if output, err := exec.Command("umount", path).CombinedOutput(); err != nil {
		t.Fatalf("disconnect: %v: %s", err, output)
	}
}

func TestMountRepeatedLease(t *testing.T) {
	for _, fs := range []string{"ext4", "xfs"} {
		t.Run(fs, func(t *testing.T) {
			m := recoveryManager(t, fs)
			path := mountRecoveryVolume(t, m, "container-one")
			writeRecoveryMarker(t, path)

			path = mountRecoveryVolume(t, m, "container-one")

			assertRecoveryMarker(t, path)
			leases, err := os.ReadDir(filepath.Join(m.stateDir, recoveryVolumeName))
			if err != nil || len(leases) != 1 {
				t.Fatalf("repeated Mount changed leases: %v, %v", leases, err)
			}
		})
	}
}

func TestMountRecoversLostMountWithSurvivingLease(t *testing.T) {
	for _, fs := range []string{"ext4", "xfs"} {
		for _, lease := range []string{"container-one", "container-two"} {
			for _, keepMountDir := range []bool{true, false} {
				name := fs + "/" + lease
				if keepMountDir {
					name += "/empty-mount-dir"
				} else {
					name += "/missing-mount-dir"
				}
				t.Run(name, func(t *testing.T) {
					m := recoveryManager(t, fs)
					path := mountRecoveryVolume(t, m, "container-one")
					writeRecoveryMarker(t, path)
					disconnectRecoveryVolume(t, path)
					if !keepMountDir {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}

					// Reconstruct the manager as a restarted plugin would.
					restarted := Manager{dataDir: m.dataDir, stateDir: m.stateDir, mountDir: m.mountDir}
					path = mountRecoveryVolume(t, restarted, lease)

					assertRecoveryMarker(t, path)
					if _, err := os.Stat(filepath.Join(m.stateDir, recoveryVolumeName, "container-one")); err != nil {
						t.Fatalf("original lease was removed: %v", err)
					}
				})
			}
		}
	}
}

func TestMountFailurePreservesExistingLeaseAndDirectory(t *testing.T) {
	for _, lease := range []string{"container-one", "container-two"} {
		t.Run(lease, func(t *testing.T) {
			m := recoveryManager(t, "ext4")
			path := mountRecoveryVolume(t, m, "container-one")
			writeRecoveryMarker(t, path)
			disconnectRecoveryVolume(t, path)
			// Recovery must refuse to hide unrelated files in a stale mount directory.
			if err := os.WriteFile(filepath.Join(path, "keep"), []byte("do not remove"), 0600); err != nil {
				t.Fatal(err)
			}

			result, err := m.Mount(context.New(), recoveryVolumeName, lease)

			if err == nil || result != "" {
				t.Fatalf("expected an explicit failure, got path %q and error %v", result, err)
			}
			if _, err := os.Stat(filepath.Join(m.stateDir, recoveryVolumeName, "container-one")); err != nil {
				t.Fatalf("existing lease was removed: %v", err)
			}
			if data, err := os.ReadFile(filepath.Join(path, "keep")); err != nil || string(data) != "do not remove" {
				t.Fatalf("stale directory data changed: %q, %v", data, err)
			}
			leases, err := os.ReadDir(filepath.Join(m.stateDir, recoveryVolumeName))
			if err != nil || len(leases) != 1 || leases[0].Name() != "container-one" {
				t.Fatalf("failed Mount changed the original leases: %v, %v", leases, err)
			}
		})
	}
}

func TestUnmountAfterRepeatedMountPreservesOtherLease(t *testing.T) {
	m := recoveryManager(t, "xfs")
	path := mountRecoveryVolume(t, m, "container-one")
	writeRecoveryMarker(t, path)
	mountRecoveryVolume(t, m, "container-one")
	mountRecoveryVolume(t, m, "container-two")

	if err := m.UnMount(context.New(), recoveryVolumeName, "container-one"); err != nil {
		t.Fatal(err)
	}
	assertRecoveryMarker(t, path)
	if err := m.UnMount(context.New(), recoveryVolumeName, "container-two"); err != nil {
		t.Fatal(err)
	}
	if mounted, err := isMountPoint(path); err != nil || mounted {
		t.Fatalf("last lease did not release the mount: mounted=%v, err=%v", mounted, err)
	}
}

func TestMountVolumeWhoseSizeIsNotASectorMultiple(t *testing.T) {
	// the validator asks for "<n>g" = n*10^9 bytes, which is never a multiple of 4096
	for _, fs := range []string{"xfs", "ext4"} {
		m := recoveryManagerWithSize(t, fs, 1000000000)
		path := mountRecoveryVolume(t, m, "container-one")
		writeRecoveryMarker(t, path)
		if err := m.UnMount(context.New(), recoveryVolumeName, "container-one"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMountFromAnotherPluginViewReusesTheLoopDevice(t *testing.T) {
	// a restarted plugin that does not see the first kernel mount must not put a second
	// filesystem instance on the same image
	for _, fs := range []string{"ext4", "xfs"} {
		t.Run(fs, func(t *testing.T) {
			m := recoveryManager(t, fs)
			path := mountRecoveryVolume(t, m, "container-one")
			writeRecoveryMarker(t, path)

			other := Manager{dataDir: m.dataDir, stateDir: m.stateDir, mountDir: t.TempDir()}
			t.Cleanup(func() {
				exec.Command("umount", filepath.Join(other.mountDir, recoveryVolumeName)).Run()
			})
			otherPath := mountRecoveryVolume(t, other, "container-two")

			assertRecoveryMarker(t, otherPath)
			devices, err := exec.Command("losetup", "--noheadings", "--output", "NAME",
				"--associated", filepath.Join(m.dataDir, recoveryVolumeName)).CombinedOutput()
			if err != nil {
				t.Fatalf("losetup: %v: %s", err, devices)
			}
			if lines := strings.Fields(string(devices)); len(lines) != 1 {
				t.Fatalf("the image has %d loop devices, want 1: %s", len(lines), devices)
			}
		})
	}
}
