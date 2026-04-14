package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"savk/internal/collectors"
	"savk/internal/contract"
	"savk/internal/engine"
	"savk/internal/reporters"
)

var (
	version                  = "0.1.0"
	commit                   = "unknown"
	buildDate                = "unknown"
	newPathChecker           = func() collectors.PathChecker { return collectors.OSPathChecker{} }
	newCommandRunner         = func() collectors.CommandRunner { return collectors.OSCommandRunner{} }
	newProcessReader         = func() collectors.ProcessReader { return collectors.OSProcessReader{} }
	newServiceNamespaceProbe = func() collectors.ServiceNamespaceProbe { return collectors.OSServiceNamespaceProbe{} }
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 3
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "version", "-v", "--version":
		printVersion(stdout)
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 3
	}
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(stderr)

	contractPath := flags.String("contract", "", "path to the SAVK contract")
	if err := flags.Parse(args); err != nil {
		return 3
	}
	if *contractPath == "" {
		fmt.Fprintln(stderr, "missing required flag --contract")
		return 3
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected extra arguments: %v\n", flags.Args())
		return 3
	}

	if _, err := contract.ParseFile(*contractPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}

	fmt.Fprintln(stdout, "contract valid")
	return 0
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)

	contractPath := flags.String("contract", "", "path to the SAVK contract")
	format := flags.String("format", "json", "output format")
	domainFilter := flags.String("domain", "", "comma-separated domains to run")
	collectorTimeout := flags.Duration("collector-timeout", 2*time.Second, "per-collector timeout")
	hostRoot := flags.String("host-root", "", "optional absolute host filesystem root for paths and sockets")
	includeRaw := flags.Bool("include-raw", false, "include full collector raw evidence in the report")
	if err := flags.Parse(args); err != nil {
		return 3
	}
	if *contractPath == "" {
		fmt.Fprintln(stderr, "missing required flag --contract")
		return 3
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected extra arguments: %v\n", flags.Args())
		return 3
	}
	if *collectorTimeout <= 0 {
		fmt.Fprintln(stderr, "--collector-timeout must be > 0")
		return 3
	}
	if err := validateHostRoot(*hostRoot); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	if *format != "json" && *format != "table" {
		fmt.Fprintf(stderr, "unsupported format %q; supported formats: json, table\n", *format)
		return 3
	}

	data, err := os.ReadFile(*contractPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	cfg, err := contract.ParseBytes(data)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}

	domains, err := selectedDomains(cfg, *domainFilter)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	if len(domains) == 0 {
		fmt.Fprintln(stderr, "no domains selected")
		return 3
	}
	if *hostRoot != "" && hasServiceBackedDomain(domains) {
		fmt.Fprintln(stderr, "--host-root is only supported for paths and sockets in v0.1")
		return 3
	}

	checks, err := buildChecksForDomains(cfg, domains, *hostRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	if len(checks) == 0 {
		fmt.Fprintln(stderr, "no checks generated from selected domains")
		return 3
	}

	startedAt := time.Now().UTC()
	results, err := engine.New().WithCollectorTimeout(*collectorTimeout).Run(context.Background(), checks)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	reportInput := reporters.JSONReportInput{
		ToolVersion:     version,
		ContractVersion: cfg.APIVersion,
		ContractHash:    hashContract(data),
		RunID:           startedAt.Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid()),
		Target:          cfg.Metadata.Target,
		Host:            host,
		StartedAt:       startedAt,
		DurationMs:      time.Since(startedAt).Milliseconds(),
		IncludeRaw:      *includeRaw,
		Results:         results,
	}

	var output []byte
	switch *format {
	case "json":
		output, err = reporters.RenderJSONReport(reportInput)
	case "table":
		output, err = reporters.RenderTableReport(reportInput)
	default:
		err = fmt.Errorf("unsupported format %q", *format)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}

	if _, err := stdout.Write(output); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}

	return reporters.ExitCodeForResults(results)
}

func buildChecksForDomains(cfg *contract.Contract, domains []string, hostRoot string) ([]engine.Check, error) {
	checks := make([]engine.Check, 0)
	pathChecker := newPathChecker()
	if hostRoot != "" {
		pathChecker = collectors.NewRootedPathChecker(hostRoot, pathChecker)
	}
	commandRunner := newCommandRunner()
	processReader := newProcessReader()
	namespaceProbe := newServiceNamespaceProbe()
	includeServices := slices.Contains(domains, "services")
	includeIdentity := slices.Contains(domains, "identity")

	for _, domain := range domains {
		switch domain {
		case "paths":
			pathChecks := collectors.BuildPathChecks(cfg.Paths, pathChecker)
			if len(pathChecks) == 0 {
				continue
			}
			if hostRoot == "" {
				checks = append(checks, collectors.NewPathNamespaceCheck(cfg.Metadata.Target, namespaceProbe))
				checks = append(checks, withPrerequisite(pathChecks, collectors.PathNamespaceCheckID)...)
			} else {
				checks = append(checks, pathChecks...)
			}
		case "sockets":
			socketChecks := collectors.BuildSocketChecks(cfg.Sockets, pathChecker)
			if len(socketChecks) == 0 {
				continue
			}
			if hostRoot == "" {
				checks = append(checks, collectors.NewSocketNamespaceCheck(cfg.Metadata.Target, namespaceProbe))
				checks = append(checks, withPrerequisite(socketChecks, collectors.SocketNamespaceCheckID)...)
			} else {
				checks = append(checks, socketChecks...)
			}
		case "identity", "services":
			continue
		default:
			return nil, fmt.Errorf("unsupported domain %q", domain)
		}
	}

	if includeServices || includeIdentity {
		serviceChecks, identityChecks, err := buildServiceBackedChecks(cfg, includeServices, includeIdentity, commandRunner, processReader)
		if err != nil {
			return nil, err
		}
		if len(serviceChecks) > 0 {
			checks = append(checks, collectors.NewServiceNamespaceCheck(cfg.Metadata.Target, namespaceProbe))
			checks = append(checks, withPrerequisite(serviceChecks, collectors.ServiceNamespaceCheckID)...)
		}
		checks = append(checks, identityChecks...)
	}

	return checks, nil
}

func buildServiceBackedChecks(cfg *contract.Contract, includeServices, includeIdentity bool, runner collectors.CommandRunner, processReader collectors.ProcessReader) ([]engine.Check, []engine.Check, error) {
	serviceChecks := make([]engine.Check, 0)
	if includeServices {
		serviceChecks = append(serviceChecks, collectors.BuildServiceChecks(cfg.Services, runner)...)
	}

	if !includeIdentity {
		return serviceChecks, nil, nil
	}

	synthesizedStates := make(map[string]contract.ServiceSpec)
	for label, spec := range cfg.Identity {
		if declared, ok := cfg.Services[spec.Service]; ok {
			if declared.State != contract.ServiceStateActive {
				return nil, nil, fmt.Errorf("identity.%s.service references %s but services.%s.state is %s; runtime identity requires active", label, spec.Service, spec.Service, declared.State)
			}
			if !includeServices {
				synthesizedStates[spec.Service] = contract.ServiceSpec{State: declared.State}
			}
			continue
		}

		synthesizedStates[spec.Service] = contract.ServiceSpec{State: contract.ServiceStateActive}
	}
	if len(synthesizedStates) > 0 {
		serviceChecks = append(serviceChecks, collectors.BuildServiceStateChecks(synthesizedStates, runner)...)
	}

	identityChecks, err := collectors.BuildIdentityChecks(cfg.Identity, runner, processReader)
	if err != nil {
		return nil, nil, err
	}

	return serviceChecks, identityChecks, nil
}

func selectedDomains(cfg *contract.Contract, filter string) ([]string, error) {
	if filter != "" {
		parts := strings.Split(filter, ",")
		domains := make([]string, 0, len(parts))
		seen := make(map[string]struct{}, len(parts))
		for _, part := range parts {
			domain := strings.TrimSpace(part)
			if domain == "" {
				continue
			}
			if _, ok := seen[domain]; ok {
				continue
			}
			seen[domain] = struct{}{}
			domains = append(domains, domain)
		}
		slices.Sort(domains)
		return domains, nil
	}

	domains := make([]string, 0, 4)
	if len(cfg.Paths) > 0 {
		domains = append(domains, "paths")
	}
	if len(cfg.Identity) > 0 {
		domains = append(domains, "identity")
	}
	if len(cfg.Sockets) > 0 {
		domains = append(domains, "sockets")
	}
	if len(cfg.Services) > 0 {
		domains = append(domains, "services")
	}
	slices.Sort(domains)
	return domains, nil
}

func hashContract(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  savk check --contract <file> [--format json|table] [--domain paths] [--collector-timeout 2s] [--host-root /host] [--include-raw]")
	fmt.Fprintln(w, "  savk validate --contract <file>")
	fmt.Fprintln(w, "  savk version")
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "savk %s\n", version)
	fmt.Fprintf(w, "commit: %s\n", commit)
	fmt.Fprintf(w, "buildDate: %s\n", buildDate)
	fmt.Fprintf(w, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "contractVersion: %s\n", contract.APIVersionV1)
	fmt.Fprintf(w, "reportSchema: %s\n", reporters.SchemaVersionV1)
}

func withPrerequisite(checks []engine.Check, prerequisite string) []engine.Check {
	wrapped := make([]engine.Check, 0, len(checks))
	for _, check := range checks {
		wrapped = append(wrapped, prerequisiteCheck{
			Check:        check,
			prerequisite: prerequisite,
		})
	}

	return wrapped
}

func validateHostRoot(root string) error {
	if root == "" {
		return nil
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("--host-root must be an absolute path")
	}

	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("--host-root %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--host-root must point to a directory")
	}

	return nil
}

func hasServiceBackedDomain(domains []string) bool {
	return slices.Contains(domains, "services") || slices.Contains(domains, "identity")
}

type prerequisiteCheck struct {
	engine.Check
	prerequisite string
}

func (c prerequisiteCheck) Prerequisites() []string {
	prerequisites := append([]string{c.prerequisite}, c.Check.Prerequisites()...)
	return slices.Compact(prerequisites)
}
