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
		{name: "empty-contract.yaml", want: "empty contract: at least one of services, sockets, paths, identity must be non-empty"},
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
		"empty-contract",
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
