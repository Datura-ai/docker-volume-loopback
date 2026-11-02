package manager

import (
	"github.com/ashald/docker-volume-loopback/context"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"
)

// fsTypePattern takes the type out of a `file -b` description: "SGI XFS filesystem data (blksz
// 4096, ...)", "Linux rev 1.0 ext4 filesystem data, UUID=..."
var fsTypePattern = regexp.MustCompile(`(\S+) filesystem data`)

type Volume struct {
	Name                 string
	AllocatedSizeInBytes uint64
	MaxSizeInBytes       uint64
	StateDir             string
	DataFilePath         string
	MountPointPath       string
	CreatedAt            time.Time
	fs                   string
}

func (v Volume) IsMounted(ctx *context.Context) (mounted bool, err error) {
	{
		ctx = ctx.
			Field(":func", "Volume/IsMounted")

		ctx.
			Level(context.Debug).
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
					Field(":return/mounted", mounted).
					Message("finished")
			}
		}()
	}

	files, err := ioutil.ReadDir(v.StateDir)
	if err != nil {
		if os.IsNotExist(err) {
			mounted = false
			err = nil
			return
		}
		err = errors.Wrapf(err,
			"error checking volume's mount status - cannot read volume state dir '%s'",
			v.StateDir)
		return
	}
	mounted = len(files) > 0
	return
}

func (v Volume) Fs(ctx *context.Context) (fs string, err error) {
	{
		ctx = ctx.
			Field(":func", "Volume/Fs")

		ctx.
			Level(context.Debug).
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
					Field(":return/fs", fs).
					Message("finished")
			}
		}()
	}

	var output string

	if len(v.fs) == 0 {
		// -b: the description without the path, which may itself contain "filesystem"
		output, err = runCommandStdout(ctx.Derived(), "file", "-b", v.DataFilePath)
		if err != nil {
			err = errors.Wrapf(err, "cannot inspect the data file '%s'", v.DataFilePath)
			return
		}
		match := fsTypePattern.FindStringSubmatch(output)
		if match == nil {
			err = errors.Errorf("cannot tell the filesystem type of '%s' from %q", v.DataFilePath, output)
			return
		}
		v.fs = strings.ToLower(match[1])
	}

	fs = v.fs
	return
}
