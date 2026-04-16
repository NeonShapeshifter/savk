package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValidFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
	}{
		{name: "minimal-paths.yaml"},
		{name: "full-contract.yaml"},
		{name: "quoted-path-key.yaml"},
		{name: "quoted-colon-path-key.yaml"},
		{name: "quoted-hash-socket-key.yaml"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := readFixture(t, "valid", tc.name)
			contract, err := ParseBytes(data)
			if err != nil {
				t.Fatalf("ParseBytes() error = %v", err)
			}

			if contract.APIVersion != APIVersionV1 {
				t.Fatalf("APIVersion = %q, want %q", contract.APIVersion, APIVersionV1)
			}
			if contract.Kind != KindApplianceContract {
				t.Fatalf("Kind = %q, want %q", contract.Kind, KindApplianceContract)
			}
			if contract.Metadata.Target != TargetLinuxSystemd {
				t.Fatalf("Metadata.Target = %q, want %q", contract.Metadata.Target, TargetLinuxSystemd)
			}
		})
	}
}

func TestParseQuotedAbsoluteKeysDecodeToUnquotedPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		wantPath   string
		wantSocket string
	}{
		{name: "quoted-path-key.yaml", wantPath: "/etc/hosts"},
		{name: "quoted-colon-path-key.yaml", wantPath: "/tmp/a:b"},
		{name: "quoted-hash-socket-key.yaml", wantSocket: "/tmp/a#b"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := ParseBytes(readFixture(t, "valid", tc.name))
			if err != nil {
				t.Fatalf("ParseBytes() error = %v", err)
			}

			if tc.wantPath != "" {
				if _, ok := cfg.Paths[tc.wantPath]; !ok {
					t.Fatalf("cfg.Paths missing key %q; got %v", tc.wantPath, keysOfPaths(cfg.Paths))
				}
			}
			if tc.wantSocket != "" {
				if _, ok := cfg.Sockets[tc.wantSocket]; !ok {
					t.Fatalf("cfg.Sockets missing key %q; got %v", tc.wantSocket, keysOfSockets(cfg.Sockets))
				}
			}
		})
	}
}

func TestParseInvalidFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
	}{
		{name: "duplicate-keys.yaml", want: `duplicate key "/etc/savk/config.yaml"`},
		{name: "tabs.yaml", want: "tab indentation is not supported"},
		{name: "flow-style.yaml", want: "flow style is not supported"},
		{name: "relative-path.yaml", want: `relative path "var/log/savk" at paths`},
		{name: "unknown-field.yaml", want: `unknown field "onwer" at paths./etc/savk/config.yaml`},
		{name: "invalid-enum.yaml", want: `invalid restart policy "on_failure"`},
		{name: "alias.yaml", want: `aliases are not supported`},
		{name: "tag.yaml", want: `tags are not supported`},
		{name: "inline-comment.yaml", want: `inline comments are not supported`},
		{name: "invalid-capability.yaml", want: `invalid capability "net_admin"`},
		{name: "identity-missing-service.yaml", want: `missing required field at identity.sensor_runtime.service`},
		{name: "identity-empty-capabilities.yaml", want: `at least one capability set must be defined at identity.sensor_runtime.capabilities`},
		{name: "identity-inactive-service.yaml", want: `references sensor-agent.service but services.sensor-agent.service.state is inactive; runtime identity requires active at identity.sensor_runtime.service`},
		{name: "empty-contract.yaml", want: "empty contract: at least one of services, sockets, paths, identity must be non-empty"},
		{name: "quoted-relative-key.yaml", want: `relative path "var/log/savk" at paths`},
		{name: "unquoted-colon-path-key.yaml", want: `unquoted mapping keys containing ':' are not supported at line 8`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := readFixture(t, "invalid", tc.name)
			_, err := ParseBytes(data)
			if err == nil {
				t.Fatal("ParseBytes() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseBytes() error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseInvalidFixturesGolden(t *testing.T) {
	t.Parallel()

	cases := []string{
		"duplicate-keys",
		"tabs",
		"flow-style",
		"relative-path",
		"unknown-field",
		"invalid-enum",
		"alias",
		"tag",
		"inline-comment",
		"invalid-capability",
		"identity-missing-service",
		"identity-empty-capabilities",
		"identity-inactive-service",
		"empty-contract",
		"quoted-relative-key",
		"unquoted-colon-path-key",
	}

	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := readFixture(t, "invalid", name+".yaml")
			_, err := ParseBytes(data)
			if err == nil {
				t.Fatal("ParseBytes() error = nil, want error")
			}

			want := readGolden(t, "parser", name+".txt")
			got := err.Error()
			if got != want {
				t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
			}
		})
	}
}

func keysOfPaths(values map[string]PathSpec) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func keysOfSockets(values map[string]SocketSpec) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestParseBytesRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	data := []byte{
		'a', 'p', 'i', 'V', 'e', 'r', 's', 'i', 'o', 'n', ':', ' ', 's', 'a', 'v', 'k', '/', 'v', '1', '\n',
		'k', 'i', 'n', 'd', ':', ' ', 'A', 'p', 'p', 'l', 'i', 'a', 'n', 'c', 'e', 'C', 'o', 'n', 't', 'r', 'a', 'c', 't', '\n',
		'm', 'e', 't', 'a', 'd', 'a', 't', 'a', ':', '\n',
		' ', ' ', 'n', 'a', 'm', 'e', ':', ' ', 0xff, '\n',
		' ', ' ', 't', 'a', 'r', 'g', 'e', 't', ':', ' ', 'l', 'i', 'n', 'u', 'x', '-', 's', 'y', 's', 't', 'e', 'm', 'd', '\n',
		'p', 'a', 't', 'h', 's', ':', '\n',
		' ', ' ', '/', 'e', 't', 'c', '/', 'h', 'o', 's', 't', 's', ':', '\n',
		' ', ' ', ' ', ' ', 't', 'y', 'p', 'e', ':', ' ', 'f', 'i', 'l', 'e', '\n',
	}

	_, err := ParseBytes(data)
	if err == nil {
		t.Fatal("ParseBytes() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "input must be valid UTF-8") {
		t.Fatalf("ParseBytes() error = %q, want UTF-8 validation message", err.Error())
	}
}

func TestParseBytesRejectsMalformedQuotedScalars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		metadataName string
	}{
		{name: "malformed double-quoted scalar", metadataName: `"bad"name"`},
		{name: "malformed single-quoted scalar", metadataName: `'bad'name'`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := []byte(strings.Join([]string{
				"apiVersion: savk/v1",
				"kind: ApplianceContract",
				"metadata:",
				"  name: " + tc.metadataName,
				"  target: linux-systemd",
				"paths:",
				"  /etc/hosts:",
				"    type: file",
				"",
			}, "\n"))

			_, err := ParseBytes(data)
			if err == nil {
				t.Fatal("ParseBytes() error = nil, want error")
			}
			if !strings.Contains(err.Error(), "unterminated quoted string") {
				t.Fatalf("ParseBytes() error = %q, want malformed-quote message", err.Error())
			}
		})
	}
}

func TestParseQuotedScalarsDecodeEscapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		metadataName string
		pathKey      string
		ownerValue   string
		wantName     string
		wantPath     string
		wantOwner    string
	}{
		{
			name:         "single quoted doubled quote",
			metadataName: `'sensor''agent'`,
			pathKey:      `'/tmp/quote''file'`,
			ownerValue:   `'ops''team'`,
			wantName:     "sensor'agent",
			wantPath:     "/tmp/quote'file",
			wantOwner:    "ops'team",
		},
		{
			name:         "double quoted escaped quote",
			metadataName: `"sensor\"agent"`,
			pathKey:      `"/tmp/quote\"file"`,
			ownerValue:   `"ops\"team"`,
			wantName:     `sensor"agent`,
			wantPath:     `/tmp/quote"file`,
			wantOwner:    `ops"team`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := []byte(strings.Join([]string{
				"apiVersion: savk/v1",
				"kind: ApplianceContract",
				"metadata:",
				"  name: " + tc.metadataName,
				"  target: linux-systemd",
				"paths:",
				"  " + tc.pathKey + ":",
				"    owner: " + tc.ownerValue,
				"    type: file",
				"",
			}, "\n"))

			cfg, err := ParseBytes(data)
			if err != nil {
				t.Fatalf("ParseBytes() error = %v", err)
			}
			if cfg.Metadata.Name != tc.wantName {
				t.Fatalf("Metadata.Name = %q, want %q", cfg.Metadata.Name, tc.wantName)
			}

			spec, ok := cfg.Paths[tc.wantPath]
			if !ok {
				t.Fatalf("cfg.Paths missing decoded key %q; got %v", tc.wantPath, keysOfPaths(cfg.Paths))
			}
			if spec.Owner != tc.wantOwner {
				t.Fatalf("spec.Owner = %q, want %q", spec.Owner, tc.wantOwner)
			}
		})
	}
}

func TestParseBytesRejectsUnsupportedQuotedEscapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lines []string
	}{
		{
			name: "unsupported double quoted value escape",
			lines: []string{
				"apiVersion: savk/v1",
				"kind: ApplianceContract",
				"metadata:",
				`  name: "bad\qname"`,
				"  target: linux-systemd",
				"paths:",
				"  /etc/hosts:",
				"    type: file",
			},
		},
		{
			name: "unsupported double quoted key escape",
			lines: []string{
				"apiVersion: savk/v1",
				"kind: ApplianceContract",
				"metadata:",
				"  name: bad-key",
				"  target: linux-systemd",
				"paths:",
				`  "/tmp/bad\qpath":`,
				"    type: file",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseBytes([]byte(strings.Join(append(tc.lines, ""), "\n")))
			if err == nil {
				t.Fatal("ParseBytes() error = nil, want error")
			}
			if !strings.Contains(err.Error(), `unsupported escape sequence \q`) {
				t.Fatalf("ParseBytes() error = %q, want unsupported-escape message", err.Error())
			}
		})
	}
}

func readFixture(t *testing.T, kind, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "fixtures", kind, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}

	return data
}

func readGolden(t *testing.T, kind, name string) string {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "golden", kind, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}

	return strings.TrimRight(string(data), "\n")
}
