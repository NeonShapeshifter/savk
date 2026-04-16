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

	passwdOnce    sync.Once
	passwdByUID   map[uint32]passwdEntry
	passwdByName  map[string]passwdEntry
	passwdUIDDup  map[uint32]struct{}
	passwdNameDup map[string]struct{}
	passwdErr     error

	groupOnce    sync.Once
	groupByGID   map[uint32]groupEntry
	groupByName  map[string]groupEntry
	groupGIDDup  map[uint32]struct{}
	groupNameDup map[string]struct{}
	groupErr     error
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
	if _, ambiguous := r.passwdUIDDup[uid]; ambiguous {
		return "", fmt.Errorf("uid %d is ambiguous in %s", uid, r.passwdPath)
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
	if _, ambiguous := r.groupGIDDup[gid]; ambiguous {
		return "", fmt.Errorf("gid %d is ambiguous in %s", gid, r.groupPath)
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
	if _, ambiguous := r.passwdNameDup[user]; ambiguous {
		return "", fmt.Errorf("user %q is ambiguous in %s", user, r.passwdPath)
	}

	entry, ok := r.passwdByName[user]
	if !ok {
		return "", fmt.Errorf("user %q is not present in %s", user, r.passwdPath)
	}

	group, err := r.GroupNameByGID(entry.gid)
	if err != nil {
		return "", fmt.Errorf("primary gid %d for user %q: %w", entry.gid, user, err)
	}

	return group, nil
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
	if err := r.loadPasswd(); err != nil {
		return "", err
	}

	literal, literalOK, err := r.lookupLiteralUserName(value)
	if err != nil {
		return "", err
	}
	byUID, byUIDOK, err := r.lookupUserNameByUID(uint32(id))
	if err != nil {
		return "", err
	}
	if literalOK && byUIDOK && literal != byUID {
		return "", fmt.Errorf("numeric user value %q is ambiguous in %s: literal name %q conflicts with uid %d -> %q", value, r.passwdPath, literal, id, byUID)
	}
	if literalOK {
		return literal, nil
	}
	if byUIDOK {
		return byUID, nil
	}

	return "", fmt.Errorf("uid %d is not present in %s", id, r.passwdPath)
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
	if err := r.loadGroup(); err != nil {
		return "", err
	}

	literal, literalOK, err := r.lookupLiteralGroupName(value)
	if err != nil {
		return "", err
	}
	byGID, byGIDOK, err := r.lookupGroupNameByGID(uint32(id))
	if err != nil {
		return "", err
	}
	if literalOK && byGIDOK && literal != byGID {
		return "", fmt.Errorf("numeric group value %q is ambiguous in %s: literal name %q conflicts with gid %d -> %q", value, r.groupPath, literal, id, byGID)
	}
	if literalOK {
		return literal, nil
	}
	if byGIDOK {
		return byGID, nil
	}

	return "", fmt.Errorf("gid %d is not present in %s", id, r.groupPath)
}

func (r *fileAccountResolver) loadPasswd() error {
	r.passwdOnce.Do(func() {
		r.passwdByUID = make(map[uint32]passwdEntry)
		r.passwdByName = make(map[string]passwdEntry)
		r.passwdUIDDup = make(map[uint32]struct{})
		r.passwdNameDup = make(map[string]struct{})

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
			if existing, exists := r.passwdByUID[entry.uid]; exists {
				if existing != entry {
					r.passwdUIDDup[entry.uid] = struct{}{}
				}
			} else {
				r.passwdByUID[entry.uid] = entry
			}
			if existing, exists := r.passwdByName[entry.name]; exists {
				if existing != entry {
					r.passwdNameDup[entry.name] = struct{}{}
				}
			} else {
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
		r.groupGIDDup = make(map[uint32]struct{})
		r.groupNameDup = make(map[string]struct{})

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
			if existing, exists := r.groupByGID[entry.gid]; exists {
				if existing != entry {
					r.groupGIDDup[entry.gid] = struct{}{}
				}
			} else {
				r.groupByGID[entry.gid] = entry
			}
			if existing, exists := r.groupByName[entry.name]; exists {
				if existing != entry {
					r.groupNameDup[entry.name] = struct{}{}
				}
			} else {
				r.groupByName[entry.name] = entry
			}
		}
		if err := scanner.Err(); err != nil {
			r.groupErr = fmt.Errorf("unable to scan %s: %w", r.groupPath, err)
		}
	})

	return r.groupErr
}

func (r *fileAccountResolver) lookupLiteralUserName(value string) (string, bool, error) {
	if _, ambiguous := r.passwdNameDup[value]; ambiguous {
		return "", false, fmt.Errorf("user %q is ambiguous in %s", value, r.passwdPath)
	}
	entry, ok := r.passwdByName[value]
	if !ok {
		return "", false, nil
	}

	return entry.name, true, nil
}

func (r *fileAccountResolver) lookupLiteralGroupName(value string) (string, bool, error) {
	if _, ambiguous := r.groupNameDup[value]; ambiguous {
		return "", false, fmt.Errorf("group %q is ambiguous in %s", value, r.groupPath)
	}
	entry, ok := r.groupByName[value]
	if !ok {
		return "", false, nil
	}

	return entry.name, true, nil
}

func (r *fileAccountResolver) lookupUserNameByUID(uid uint32) (string, bool, error) {
	if _, ambiguous := r.passwdUIDDup[uid]; ambiguous {
		return "", false, fmt.Errorf("uid %d is ambiguous in %s", uid, r.passwdPath)
	}

	entry, ok := r.passwdByUID[uid]
	if !ok {
		return "", false, nil
	}

	return entry.name, true, nil
}

func (r *fileAccountResolver) lookupGroupNameByGID(gid uint32) (string, bool, error) {
	if _, ambiguous := r.groupGIDDup[gid]; ambiguous {
		return "", false, fmt.Errorf("gid %d is ambiguous in %s", gid, r.groupPath)
	}

	entry, ok := r.groupByGID[gid]
	if !ok {
		return "", false, nil
	}

	return entry.name, true, nil
}
