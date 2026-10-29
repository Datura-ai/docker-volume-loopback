package manager

import (
	"os"
	"path/filepath"
	"testing"
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
