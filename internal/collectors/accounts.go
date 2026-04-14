package collectors

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type AccountResolver interface {
	UserNameByUID(uid uint32) (string, error)
	GroupNameByGID(gid uint32) (string, error)
	PrimaryGroupNameByUser(user string) (string, error)
	NormalizeUserValue(value string) (string, error)
	NormalizeGroupValue(value string) (string, error)
	PasswdPath() string
	GroupPath() string
}

type passwdEntry struct {
	name string
	uid  uint32
	gid  uint32
}

type groupEntry struct {
	name string
	gid  uint32
}

type fileAccountResolver struct {
	passwdPath string
	groupPath  string

	passwdOnce   sync.Once
	passwdByUID  map[uint32]passwdEntry
	passwdByName map[string]passwdEntry
	passwdErr    error

	groupOnce   sync.Once
	groupByGID  map[uint32]groupEntry
	groupByName map[string]groupEntry
	groupErr    error
}

func NewAccountResolver(root string) AccountResolver {
	passwdPath := "/etc/passwd"
	groupPath := "/etc/group"
	if root != "" {
		cleanRoot := filepath.Clean(root)
		passwdPath = filepath.Join(cleanRoot, "etc", "passwd")
		groupPath = filepath.Join(cleanRoot, "etc", "group")
	}

	return &fileAccountResolver{
		passwdPath: passwdPath,
		groupPath:  groupPath,
	}
}

func accountResolverForPathChecker(checker PathChecker) AccountResolver {
	switch value := checker.(type) {
	case RootedPathChecker:
		return NewAccountResolver(value.root)
	case *RootedPathChecker:
		return NewAccountResolver(value.root)
	default:
		return NewAccountResolver("")
	}
}

func (r *fileAccountResolver) PasswdPath() string {
	return r.passwdPath
}

func (r *fileAccountResolver) GroupPath() string {
	return r.groupPath
}

func (r *fileAccountResolver) UserNameByUID(uid uint32) (string, error) {
	if err := r.loadPasswd(); err != nil {
		return "", err
	}

	entry, ok := r.passwdByUID[uid]
	if !ok {
		return "", fmt.Errorf("uid %d is not present in %s", uid, r.passwdPath)
	}
	return entry.name, nil
}

func (r *fileAccountResolver) GroupNameByGID(gid uint32) (string, error) {
	if err := r.loadGroup(); err != nil {
		return "", err
	}

	entry, ok := r.groupByGID[gid]
	if !ok {
		return "", fmt.Errorf("gid %d is not present in %s", gid, r.groupPath)
	}
	return entry.name, nil
}

func (r *fileAccountResolver) PrimaryGroupNameByUser(user string) (string, error) {
	if err := r.loadPasswd(); err != nil {
		return "", err
	}
	if err := r.loadGroup(); err != nil {
		return "", err
	}

	entry, ok := r.passwdByName[user]
	if !ok {
		return "", fmt.Errorf("user %q is not present in %s", user, r.passwdPath)
	}

	group, ok := r.groupByGID[entry.gid]
	if !ok {
		return "", fmt.Errorf("primary gid %d for user %q is not present in %s", entry.gid, user, r.groupPath)
	}

	return group.name, nil
}

func (r *fileAccountResolver) NormalizeUserValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !isNumericIdentifier(value) {
		return value, nil
	}

	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid numeric uid %q", value)
	}

	return r.UserNameByUID(uint32(id))
}

func (r *fileAccountResolver) NormalizeGroupValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !isNumericIdentifier(value) {
		return value, nil
	}

	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return "", fmt.Errorf("invalid numeric gid %q", value)
	}

	return r.GroupNameByGID(uint32(id))
}

func (r *fileAccountResolver) loadPasswd() error {
	r.passwdOnce.Do(func() {
		r.passwdByUID = make(map[uint32]passwdEntry)
		r.passwdByName = make(map[string]passwdEntry)

		file, err := os.Open(r.passwdPath)
		if err != nil {
			r.passwdErr = fmt.Errorf("unable to read %s: %w", r.passwdPath, err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.Split(line, ":")
			if len(parts) < 4 {
				r.passwdErr = fmt.Errorf("invalid passwd entry at %s:%d", r.passwdPath, lineNo)
				return
			}

			name := strings.TrimSpace(parts[0])
			if name == "" {
				r.passwdErr = fmt.Errorf("invalid passwd entry at %s:%d", r.passwdPath, lineNo)
				return
			}

			uid, err := strconv.ParseUint(parts[2], 10, 32)
			if err != nil {
				r.passwdErr = fmt.Errorf("invalid uid in %s:%d: %w", r.passwdPath, lineNo, err)
				return
			}
			gid, err := strconv.ParseUint(parts[3], 10, 32)
			if err != nil {
				r.passwdErr = fmt.Errorf("invalid gid in %s:%d: %w", r.passwdPath, lineNo, err)
				return
			}

			entry := passwdEntry{
				name: name,
				uid:  uint32(uid),
				gid:  uint32(gid),
			}
			if _, exists := r.passwdByUID[entry.uid]; !exists {
				r.passwdByUID[entry.uid] = entry
			}
			if _, exists := r.passwdByName[entry.name]; !exists {
				r.passwdByName[entry.name] = entry
			}
		}
		if err := scanner.Err(); err != nil {
			r.passwdErr = fmt.Errorf("unable to scan %s: %w", r.passwdPath, err)
		}
	})

	return r.passwdErr
}

func (r *fileAccountResolver) loadGroup() error {
	r.groupOnce.Do(func() {
		r.groupByGID = make(map[uint32]groupEntry)
		r.groupByName = make(map[string]groupEntry)

		file, err := os.Open(r.groupPath)
		if err != nil {
			r.groupErr = fmt.Errorf("unable to read %s: %w", r.groupPath, err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.Split(line, ":")
			if len(parts) < 3 {
				r.groupErr = fmt.Errorf("invalid group entry at %s:%d", r.groupPath, lineNo)
				return
			}

			name := strings.TrimSpace(parts[0])
			if name == "" {
				r.groupErr = fmt.Errorf("invalid group entry at %s:%d", r.groupPath, lineNo)
				return
			}

			gid, err := strconv.ParseUint(parts[2], 10, 32)
			if err != nil {
				r.groupErr = fmt.Errorf("invalid gid in %s:%d: %w", r.groupPath, lineNo, err)
				return
			}

			entry := groupEntry{
				name: name,
				gid:  uint32(gid),
			}
			if _, exists := r.groupByGID[entry.gid]; !exists {
				r.groupByGID[entry.gid] = entry
			}
			if _, exists := r.groupByName[entry.name]; !exists {
				r.groupByName[entry.name] = entry
			}
		}
		if err := scanner.Err(); err != nil {
			r.groupErr = fmt.Errorf("unable to scan %s: %w", r.groupPath, err)
		}
	})

	return r.groupErr
}
