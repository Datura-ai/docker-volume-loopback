package driver

import (
	"testing"
	"time"

	"github.com/ashald/docker-volume-loopback/context"
	v "github.com/docker/go-plugins-helpers/volume"
)

func TestDriverMethodsShareOneLock(t *testing.T) {
	// value receivers locked a copy of the embedded mutex, so two requests never waited for each other
	root := t.TempDir()
	driver, err := New(context.New(), Config{
		StateDir: root + "/state", DataDir: root + "/data", MountDir: root + "/mnt", DefaultSize: "1G"})
	if err != nil {
		t.Fatal(err)
	}
	driver.Lock()
	finished := make(chan struct{})
	go func() {
		_, _ = driver.Get(&v.GetRequest{Name: "absent"})
		close(finished)
	}()

	select {
	case <-finished:
		t.Fatal("Get did not wait for the lock held on the driver")
	case <-time.After(200 * time.Millisecond):
	}
	driver.Unlock()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Get did not run after the lock was released")
	}
}
