package contract

type Contract struct {
	APIVersion string
	Kind       string
	Metadata   Metadata
	Services   map[string]ServiceSpec
	Sockets    map[string]SocketSpec
	Paths      map[string]PathSpec
	Identity   map[string]IdentitySpec
}

type Metadata struct {
	Name   string
	Target string
}

type ServiceSpec struct {
	State        ServiceState
	RunAs        *RunAsSpec
	Restart      *RestartPolicy
	Capabilities []string
}

type RunAsSpec struct {
	User  string
	Group string
}

type SocketSpec struct {
	Owner string
	Group string
	Mode  string
}

type PathSpec struct {
	Owner string
	Group string
	Mode  string
	Type  PathType
}

type IdentitySpec struct {
	Service      string
	UID          *int
	GID          *int
	Capabilities *CapabilitySetSpec
}

type CapabilitySetSpec struct {
	Effective   []string
	Permitted   []string
	Inheritable []string
	Bounding    []string
	Ambient     []string
}

type ServiceState string

const (
	ServiceStateActive   ServiceState = "active"
	ServiceStateInactive ServiceState = "inactive"
	ServiceStateFailed   ServiceState = "failed"
)

func ValidServiceState(value string) bool {
	switch ServiceState(value) {
	case ServiceStateActive, ServiceStateInactive, ServiceStateFailed:
		return true
	default:
		return false
	}
}

type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "always"
	RestartPolicyOnFailure RestartPolicy = "on-failure"
	RestartPolicyNo        RestartPolicy = "no"
)

func ValidRestartPolicy(value string) bool {
	switch RestartPolicy(value) {
	case RestartPolicyAlways, RestartPolicyOnFailure, RestartPolicyNo:
		return true
	default:
		return false
	}
}

type PathType string

const (
	PathTypeFile      PathType = "file"
	PathTypeDirectory PathType = "directory"
)

func ValidPathType(value string) bool {
	switch PathType(value) {
	case PathTypeFile, PathTypeDirectory:
		return true
	default:
		return false
	}
}

const (
	APIVersionV1          = "savk/v1"
	KindApplianceContract = "ApplianceContract"
	TargetLinuxSystemd    = "linux-systemd"
)
