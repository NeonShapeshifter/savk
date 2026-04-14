package collectors

import (
	"os"
	"path/filepath"
	"strings"
)

type RootedPathChecker struct {
	root string
	base PathChecker
}

func NewRootedPathChecker(root string, base PathChecker) PathChecker {
	if base == nil {
		base = OSPathChecker{}
	}

	return RootedPathChecker{
		root: filepath.Clean(root),
		base: base,
	}
}

func (c RootedPathChecker) Stat(name string) (os.FileInfo, error) {
	return c.base.Stat(c.ResolvePath(name))
}

func (c RootedPathChecker) Lstat(name string) (os.FileInfo, error) {
	return c.base.Lstat(c.ResolvePath(name))
}

func (c RootedPathChecker) ResolvePath(name string) string {
	clean := filepath.Clean(name)
	if clean == string(os.PathSeparator) {
		return c.root
	}

	trimmed := strings.TrimPrefix(clean, string(os.PathSeparator))
	return filepath.Join(c.root, trimmed)
}

func resolvedPath(checker PathChecker, name string) string {
	if checker == nil {
		return name
	}
	if resolver, ok := checker.(PathResolver); ok {
		return resolver.ResolvePath(name)
	}

	return name
}
