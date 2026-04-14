package capabilities

import (
	"sort"
	"strings"
)

var linuxCapabilityNames = []string{
	"CAP_CHOWN",
	"CAP_DAC_OVERRIDE",
	"CAP_DAC_READ_SEARCH",
	"CAP_FOWNER",
	"CAP_FSETID",
	"CAP_KILL",
	"CAP_SETGID",
	"CAP_SETUID",
	"CAP_SETPCAP",
	"CAP_LINUX_IMMUTABLE",
	"CAP_NET_BIND_SERVICE",
	"CAP_NET_BROADCAST",
	"CAP_NET_ADMIN",
	"CAP_NET_RAW",
	"CAP_IPC_LOCK",
	"CAP_IPC_OWNER",
	"CAP_SYS_MODULE",
	"CAP_SYS_RAWIO",
	"CAP_SYS_CHROOT",
	"CAP_SYS_PTRACE",
	"CAP_SYS_PACCT",
	"CAP_SYS_ADMIN",
	"CAP_SYS_BOOT",
	"CAP_SYS_NICE",
	"CAP_SYS_RESOURCE",
	"CAP_SYS_TIME",
	"CAP_SYS_TTY_CONFIG",
	"CAP_MKNOD",
	"CAP_LEASE",
	"CAP_AUDIT_WRITE",
	"CAP_AUDIT_CONTROL",
	"CAP_SETFCAP",
	"CAP_MAC_OVERRIDE",
	"CAP_MAC_ADMIN",
	"CAP_SYSLOG",
	"CAP_WAKE_ALARM",
	"CAP_BLOCK_SUSPEND",
	"CAP_AUDIT_READ",
	"CAP_PERFMON",
	"CAP_BPF",
	"CAP_CHECKPOINT_RESTORE",
}

var linuxCapabilitySet = func() map[string]struct{} {
	values := make(map[string]struct{}, len(linuxCapabilityNames))
	for _, name := range linuxCapabilityNames {
		values[name] = struct{}{}
	}
	return values
}()

func LinuxCapabilityName(bit int) string {
	if bit >= 0 && bit < len(linuxCapabilityNames) {
		return linuxCapabilityNames[bit]
	}

	return ""
}

func IsCanonical(value string) bool {
	_, ok := linuxCapabilitySet[value]
	return ok
}

func CanonicalizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ToUpper(value)
	if !strings.HasPrefix(value, "CAP_") {
		value = "CAP_" + value
	}
	return value
}

func SortCanonical(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return cloned
}

func NormalizeObserved(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ' ' || r == ',' || r == '\t'
		}) {
			token = CanonicalizeToken(token)
			if token == "" {
				continue
			}
			normalized = append(normalized, token)
		}
	}

	sort.Strings(normalized)
	return normalized
}
