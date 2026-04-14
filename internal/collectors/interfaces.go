package collectors

import (
	"context"
	"os"
)

type PathChecker interface {
	Stat(name string) (os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
}

type PathResolver interface {
	ResolvePath(name string) string
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type CommandRunner interface {
	Run(ctx context.Context, argv []string) (CommandResult, error)
}

type ProcessStatus struct {
	UID         int
	GID         int
	Effective   []string
	Permitted   []string
	Inheritable []string
	Bounding    []string
	Ambient     []string
	Cgroups     []string
	Raw         string
}

type ProcessReader interface {
	ReadStatus(ctx context.Context, pid int) (ProcessStatus, error)
}
