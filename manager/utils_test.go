package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashald/docker-volume-loopback/context"
)

func TestIsMountPoint(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		mounted   bool
		wantError bool
	}{
		{name: "mounted", command: "printf '/mnt/recovery-test\\n'", mounted: true},
		{name: "not mounted", command: "exit 1"},
		{name: "inspection failed", command: "echo 'cannot read mount table' >&2; exit 1", wantError: true},
		{name: "unexpected exit", command: "exit 2", wantError: true},
		{name: "empty success", command: "exit 0", wantError: true},
		{name: "missing command", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			if tt.command != "" {
				if err := os.WriteFile(filepath.Join(bin, "findmnt"), []byte("#!/bin/sh\n"+tt.command+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)

			mounted, err := isMountPoint("/mnt/recovery-test")

			if mounted != tt.mounted || (err != nil) != tt.wantError {
				t.Fatalf("got mounted=%v, err=%v; want mounted=%v, error=%v", mounted, err, tt.mounted, tt.wantError)
			}
		})
	}
}

func TestLoopDeviceFromLosetupTakesTheDevicePathOnly(t *testing.T) {
	for name, tc := range map[string]struct {
		output string
		want   string
		fails  bool
	}{
		"plain":            {output: "/dev/loop7\n", want: "/dev/loop7"},
		"warning first":    {output: "losetup: /data/v.img: Warning: file does not fit into a 4096-byte sector\n/dev/loop12", want: "/dev/loop12"},
		"empty":            {output: "", fails: true},
		"not a device":     {output: "losetup: cannot find an unused loop device", fails: true},
		"device then junk": {output: "/dev/loop3\nsomething else", fails: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := loopDeviceFromLosetup(tc.output)
			if tc.fails {
				if err == nil {
					t.Fatalf("accepted %q as %q", tc.output, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestMountBelongsToDataFileReadsTheSourceFromStdoutOnly(t *testing.T) {
	tests := []struct {
		name      string
		findmnt   string
		belongs   bool
		wantError bool
	}{
		{name: "same device", findmnt: "printf '/dev/loop7\\n'", belongs: true},
		{name: "warning on stderr", findmnt: "echo 'findmnt: some warning' >&2; printf '/dev/loop7\\n'", belongs: true},
		{name: "foreign device", findmnt: "printf '/dev/loop8\\n'"},
		{name: "findmnt failed", findmnt: "echo 'cannot read mount table' >&2; exit 1", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			for name, body := range map[string]string{"findmnt": tt.findmnt, "losetup": "printf '/dev/loop7\\n'"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)

			belongs, err := mountBelongsToDataFile("/mnt/recovery-test", "/data/recovery-test")

			if belongs != tt.belongs || (err != nil) != tt.wantError {
				t.Fatalf("got belongs=%v, err=%v; want belongs=%v, error=%v", belongs, err, tt.belongs, tt.wantError)
			}
			if tt.wantError && !strings.Contains(err.Error(), "cannot read mount table") {
				t.Fatalf("error does not carry findmnt's stderr: %v", err)
			}
		})
	}
}

func TestDetachStrayLoopsDetachesOnlyTheUnmountedDevices(t *testing.T) {
	tests := []struct {
		name      string
		losetup   string
		findmnt   string
		detached  []string
		wantError bool
	}{
		{name: "no devices", losetup: "exit 0"},
		{name: "stray device", losetup: "printf '/dev/loop7\\n'", findmnt: "exit 1", detached: []string{"/dev/loop7"}},
		{name: "mounted device", losetup: "printf '/dev/loop7\\n'", findmnt: "printf '/mnt/v\\n'"},
		{name: "one of two stray", losetup: "printf '/dev/loop7\\n/dev/loop8\\n'",
			findmnt: "[ \"$4\" = /dev/loop7 ] && { printf '/mnt/v\\n'; exit 0; }; exit 1", detached: []string{"/dev/loop8"}},
		{name: "losetup failed", losetup: "echo 'losetup: cannot open' >&2; exit 1", wantError: true},
		{name: "findmnt failed", losetup: "printf '/dev/loop7\\n'", findmnt: "echo 'cannot read mount table' >&2; exit 1", wantError: true},
		{name: "not a device", losetup: "printf 'junk\\n'", findmnt: "exit 1", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			detachLog := filepath.Join(bin, "detached")
			// `losetup -d <device>` is logged; every other losetup call runs the case's listing
			losetup := "if [ \"$1\" = -d ]; then echo \"$2\" >> " + detachLog + "; exit 0; fi\n" + tt.losetup
			for name, body := range map[string]string{"losetup": losetup, "findmnt": tt.findmnt} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)

			err := detachStrayLoops(context.New(), "/data/recovery-test")

			if (err != nil) != tt.wantError {
				t.Fatalf("got err=%v; want error=%v", err, tt.wantError)
			}
			logged, _ := os.ReadFile(detachLog)
			if got := strings.Fields(string(logged)); strings.Join(got, " ") != strings.Join(tt.detached, " ") {
				t.Fatalf("detached %v; want %v", got, tt.detached)
			}
		})
	}
}
