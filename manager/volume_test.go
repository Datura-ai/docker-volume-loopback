package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashald/docker-volume-loopback/context"
)

func TestFsTakesTheTypeFromTheBriefFileDescription(t *testing.T) {
	// without -b `file` prefixes the description with the path, and a path that contains
	// "filesystem" (volume "my-filesystem") was cut as the type
	tests := []struct {
		name        string
		description string
		fileExit    string
		want        string
		wantError   bool
	}{
		{name: "xfs", description: "SGI XFS filesystem data (blksz 4096, inosz 512, v2 dirs)", want: "xfs"},
		{name: "ext4", description: "Linux rev 1.0 ext4 filesystem data, UUID=0b5e0e8c-6b8f-4d9f-9b21-6a2b9d1c3e4f (extents) (64bit) (large files) (huge files)", want: "ext4"},
		{name: "no filesystem", description: "data", wantError: true},
		{name: "file failed", fileExit: "echo 'file: cannot open' >&2; exit 1", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			// the real shape: `file PATH` prints "PATH: DESCRIPTION", `file -b PATH` the description only
			fake := "#!/bin/sh\n" + tt.fileExit + "\ncase \"$1\" in -b) printf '%s\\n' '" + tt.description +
				"' ;; *) printf '%s: %s\\n' \"$1\" '" + tt.description + "' ;; esac\n"
			if err := os.WriteFile(filepath.Join(bin, "file"), []byte(fake), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			volume := Volume{DataFilePath: "/data/my-filesystem"}

			fs, err := volume.Fs(context.New())

			if fs != tt.want || (err != nil) != tt.wantError {
				t.Fatalf("got fs=%q, err=%v; want fs=%q, error=%v", fs, err, tt.want, tt.wantError)
			}
			if tt.fileExit != "" && !strings.Contains(err.Error(), "cannot open") {
				t.Fatalf("error does not carry file's stderr: %v", err)
			}
		})
	}
}
