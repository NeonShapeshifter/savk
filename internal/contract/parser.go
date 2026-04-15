package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"savk/internal/capabilities"
)

type nodeKind int

const (
	nodeMap nodeKind = iota + 1
	nodeList
	nodeString
	nodeInt
	nodeBool
)

type node struct {
	kind       nodeKind
	line       int
	stringVal  string
	intVal     int
	boolVal    bool
	mapEntries map[string]*node
	listItems  []*node
}

type sourceLine struct {
	number  int
	indent  int
	content string
}

func ParseFile(path string) (*Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseBytes(data)
}

func ParseBytes(data []byte) (*Contract, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("input must be valid UTF-8")
	}

	lines, err := tokenize(data)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty contract: at least one of services, sockets, paths, identity must be non-empty")
	}
	if lines[0].indent != 0 {
		return nil, fmt.Errorf("root indentation must start at column 0 (line %d)", lines[0].number)
	}

	root, next, err := parseBlock(lines, 0, 0)
	if err != nil {
		return nil, err
	}
	if next != len(lines) {
		return nil, fmt.Errorf("unexpected content after root block at line %d", lines[next].number)
	}
	if root.kind != nodeMap {
		return nil, fmt.Errorf("root document must be a mapping")
	}

	contract, err := decodeContract(root)
	if err != nil {
		return nil, err
	}
	if err := ValidateSemantics(contract); err != nil {
		return nil, err
	}

	return contract, nil
}

func tokenize(data []byte) ([]sourceLine, error) {
	rawLines := strings.Split(string(data), "\n")
	lines := make([]sourceLine, 0, len(rawLines))

	for i, raw := range rawLines {
		lineNo := i + 1
		raw = strings.TrimSuffix(raw, "\r")

		if strings.Contains(raw, "\t") {
			return nil, fmt.Errorf("tab indentation is not supported at line %d", lineNo)
		}

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "---" || trimmed == "..." {
			return nil, fmt.Errorf("multiple YAML documents are not supported at line %d", lineNo)
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		content := raw[indent:]
		if hasUnsupportedInlineComment(content) {
			return nil, fmt.Errorf("inline comments are not supported at line %d", lineNo)
		}
		lines = append(lines, sourceLine{
			number:  lineNo,
			indent:  indent,
			content: content,
		})
	}

	return lines, nil
}

func parseBlock(lines []sourceLine, index, indent int) (*node, int, error) {
	if index >= len(lines) {
		return nil, index, fmt.Errorf("unexpected end of input")
	}
	if lines[index].indent != indent {
		return nil, index, fmt.Errorf("unexpected indentation at line %d", lines[index].number)
	}

	content := strings.TrimSpace(lines[index].content)
	if strings.HasPrefix(content, "- ") {
		return parseList(lines, index, indent)
	}

	return parseMap(lines, index, indent)
}

func parseMap(lines []sourceLine, index, indent int) (*node, int, error) {
	start := index
	entries := make(map[string]*node)

	for index < len(lines) {
		line := lines[index]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, index, fmt.Errorf("unexpected indentation at line %d", line.number)
		}

		content := strings.TrimSpace(line.content)
		if strings.HasPrefix(content, "- ") {
			return nil, index, fmt.Errorf("expected mapping entry at line %d", line.number)
		}

		key, rawValue, err := splitKeyValue(content, line.number)
		if err != nil {
			return nil, index, err
		}
		if _, exists := entries[key]; exists {
			return nil, index, fmt.Errorf("duplicate key %q at line %d", key, line.number)
		}

		var value *node
		if rawValue == "" {
			if index+1 < len(lines) && lines[index+1].indent > indent {
				value, index, err = parseBlock(lines, index+1, lines[index+1].indent)
				if err != nil {
					return nil, index, err
				}
			} else {
				value = &node{
					kind:       nodeMap,
					line:       line.number,
					mapEntries: map[string]*node{},
				}
				index++
			}
		} else {
			value, err = parseInlineValue(rawValue, line.number)
			if err != nil {
				return nil, index, err
			}
			index++
		}

		entries[key] = value
	}

	return &node{
		kind:       nodeMap,
		line:       lines[start].number,
		mapEntries: entries,
	}, index, nil
}

func parseList(lines []sourceLine, index, indent int) (*node, int, error) {
	start := index
	items := make([]*node, 0)

	for index < len(lines) {
		line := lines[index]
		if line.indent < indent {
			break
		}
		if line.indent > indent {
			return nil, index, fmt.Errorf("unexpected indentation at line %d", line.number)
		}

		content := strings.TrimSpace(line.content)
		if !strings.HasPrefix(content, "- ") {
			break
		}

		rawValue := strings.TrimSpace(strings.TrimPrefix(content, "- "))
		if rawValue == "" {
			return nil, index, fmt.Errorf("nested list items are not supported at line %d", line.number)
		}

		value, err := parseInlineValue(rawValue, line.number)
		if err != nil {
			return nil, index, err
		}
		if value.kind == nodeMap || value.kind == nodeList {
			return nil, index, fmt.Errorf("only scalar list items are supported at line %d", line.number)
		}

		items = append(items, value)
		index++
	}

	return &node{
		kind:      nodeList,
		line:      lines[start].number,
		listItems: items,
	}, index, nil
}

func parseInlineValue(raw string, line int) (*node, error) {
	switch raw {
	case "[]":
		return &node{
			kind:      nodeList,
			line:      line,
			listItems: []*node{},
		}, nil
	case "|", ">":
		return nil, fmt.Errorf("multiline strings are not supported at line %d", line)
	}

	if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") {
		return nil, fmt.Errorf("flow style is not supported at line %d", line)
	}
	if strings.HasPrefix(raw, "|") || strings.HasPrefix(raw, ">") {
		return nil, fmt.Errorf("multiline strings are not supported at line %d", line)
	}
	if err := rejectUnsupportedScalarFeature(raw, line); err != nil {
		return nil, err
	}

	if value, quoted, err := parseQuotedString(raw, line); err != nil {
		return nil, err
	} else if quoted {
		return &node{
			kind:      nodeString,
			line:      line,
			stringVal: value,
		}, nil
	}

	switch raw {
	case "true":
		return &node{kind: nodeBool, line: line, boolVal: true}, nil
	case "false":
		return &node{kind: nodeBool, line: line, boolVal: false}, nil
	}

	if value, err := strconv.Atoi(raw); err == nil {
		return &node{kind: nodeInt, line: line, intVal: value}, nil
	}

	return &node{
		kind:      nodeString,
		line:      line,
		stringVal: raw,
	}, nil
}

func splitKeyValue(content string, line int) (string, string, error) {
	index, sawPlainColon, err := keyValueDelimiter(content, line)
	if err != nil {
		return "", "", err
	}
	if index < 0 {
		return "", "", fmt.Errorf("expected key:value pair at line %d", line)
	}

	rawKey := strings.TrimSpace(content[:index])
	if rawKey == "" {
		return "", "", fmt.Errorf("empty mapping key at line %d", line)
	}

	key, quoted, err := parseQuotedString(rawKey, line)
	if err != nil {
		return "", "", err
	}
	if !quoted {
		if sawPlainColon {
			return "", "", fmt.Errorf("unquoted mapping keys containing ':' are not supported at line %d\n  hint: quote the key to preserve it literally", line)
		}
		if err := rejectUnsupportedKeyFeature(key, line); err != nil {
			return "", "", err
		}
	}

	return key, strings.TrimSpace(content[index+1:]), nil
}

func keyValueDelimiter(content string, line int) (int, bool, error) {
	inSingle := false
	inDouble := false
	escaped := false
	sawPlainColon := false

	for i := 0; i < len(content); i++ {
		ch := content[i]
		switch ch {
		case '\\':
			if inDouble {
				escaped = !escaped
			} else {
				escaped = false
			}
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			escaped = false
		case '"':
			if !inSingle && !escaped {
				inDouble = !inDouble
			} else {
				escaped = false
			}
		case ':':
			if inSingle || inDouble {
				escaped = false
				continue
			}
			if i+1 == len(content) || content[i+1] == ' ' {
				return i, sawPlainColon, nil
			}
			sawPlainColon = true
			escaped = false
		default:
			escaped = false
		}
	}

	if inSingle || inDouble {
		return -1, false, fmt.Errorf("unterminated quoted string at line %d", line)
	}

	return -1, sawPlainColon, nil
}

func parseQuotedString(raw string, line int) (string, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return raw[1 : len(raw)-1], true, nil
	}
	if raw == `"` || raw == `'` || raw[0] == '"' || raw[0] == '\'' || raw[len(raw)-1] == '"' || raw[len(raw)-1] == '\'' {
		return "", false, fmt.Errorf("unterminated quoted string at line %d", line)
	}

	return raw, false, nil
}

func decodeContract(root *node) (*Contract, error) {
	if err := rejectUnknownFields(root, nil, []string{"apiVersion", "kind", "metadata", "services", "sockets", "paths", "identity"}); err != nil {
		return nil, err
	}

	apiVersion, err := requireStringField(root, nil, "apiVersion")
	if err != nil {
		return nil, err
	}
	if apiVersion != APIVersionV1 {
		return nil, fmt.Errorf("unsupported apiVersion %q\n  supported: %s", apiVersion, APIVersionV1)
	}

	kind, err := requireStringField(root, nil, "kind")
	if err != nil {
		return nil, err
	}
	if kind != KindApplianceContract {
		return nil, errorAt([]string{"kind"}, "invalid kind %q\n  valid value: %s", kind, KindApplianceContract)
	}

	metadataNode, err := requireMapField(root, nil, "metadata")
	if err != nil {
		return nil, err
	}
	metadata, err := decodeMetadata(metadataNode, []string{"metadata"})
	if err != nil {
		return nil, err
	}

	contract := &Contract{
		APIVersion: apiVersion,
		Kind:       kind,
		Metadata:   metadata,
	}

	if servicesNode, ok := root.mapEntries["services"]; ok {
		contract.Services, err = decodeServices(servicesNode, []string{"services"})
		if err != nil {
			return nil, err
		}
	}
	if socketsNode, ok := root.mapEntries["sockets"]; ok {
		contract.Sockets, err = decodeSockets(socketsNode, []string{"sockets"})
		if err != nil {
			return nil, err
		}
	}
	if pathsNode, ok := root.mapEntries["paths"]; ok {
		contract.Paths, err = decodePaths(pathsNode, []string{"paths"})
		if err != nil {
			return nil, err
		}
	}
	if identityNode, ok := root.mapEntries["identity"]; ok {
		contract.Identity, err = decodeIdentity(identityNode, []string{"identity"})
		if err != nil {
			return nil, err
		}
	}

	if len(contract.Services) == 0 && len(contract.Sockets) == 0 && len(contract.Paths) == 0 && len(contract.Identity) == 0 {
		return nil, fmt.Errorf("empty contract: at least one of services, sockets, paths, identity must be non-empty")
	}

	return contract, nil
}

func decodeMetadata(node *node, path []string) (Metadata, error) {
	if node.kind != nodeMap {
		return Metadata{}, errorAt(path, "expected mapping")
	}
	if err := rejectUnknownFields(node, path, []string{"name", "target"}); err != nil {
		return Metadata{}, err
	}

	name, err := requireStringField(node, path, "name")
	if err != nil {
		return Metadata{}, err
	}
	if strings.TrimSpace(name) == "" {
		return Metadata{}, errorAt(append(path, "name"), "value must not be empty")
	}

	target, err := requireStringField(node, path, "target")
	if err != nil {
		return Metadata{}, err
	}
	if target != TargetLinuxSystemd {
		return Metadata{}, fmt.Errorf("unsupported target %q\n  supported targets: %s", target, TargetLinuxSystemd)
	}

	return Metadata{
		Name:   name,
		Target: target,
	}, nil
}

func decodeServices(node *node, path []string) (map[string]ServiceSpec, error) {
	if node.kind != nodeMap {
		return nil, errorAt(path, "expected mapping")
	}

	services := make(map[string]ServiceSpec, len(node.mapEntries))
	for _, name := range sortedKeys(node.mapEntries) {
		if strings.TrimSpace(name) == "" {
			return nil, errorAt(path, "service name must not be empty")
		}

		servicePath := append(path, name)
		specNode := node.mapEntries[name]
		if specNode.kind != nodeMap {
			return nil, errorAt(servicePath, "expected mapping")
		}
		if err := rejectUnknownFields(specNode, servicePath, []string{"state", "run_as", "restart", "capabilities"}); err != nil {
			return nil, err
		}

		state, err := requireStringField(specNode, servicePath, "state")
		if err != nil {
			return nil, err
		}
		if !ValidServiceState(state) {
			return nil, errorAt(servicePath, "invalid state %q\n  valid values: active, inactive, failed", state)
		}

		spec := ServiceSpec{
			State: ServiceState(state),
		}

		if runAsNode, ok := specNode.mapEntries["run_as"]; ok {
			runAs, err := decodeRunAs(runAsNode, append(servicePath, "run_as"))
			if err != nil {
				return nil, err
			}
			spec.RunAs = &runAs
		}

		if restartNode, ok := specNode.mapEntries["restart"]; ok {
			restart, err := expectString(restartNode, append(servicePath, "restart"))
			if err != nil {
				return nil, err
			}
			if !ValidRestartPolicy(restart) {
				return nil, errorAt(servicePath, "invalid restart policy %q\n  valid values: always, on-failure, no", restart)
			}
			policy := RestartPolicy(restart)
			spec.Restart = &policy
		}

		if capsNode, ok := specNode.mapEntries["capabilities"]; ok {
			caps, err := decodeCapabilityList(capsNode, append(servicePath, "capabilities"))
			if err != nil {
				return nil, err
			}
			spec.Capabilities = caps
		}

		services[name] = spec
	}

	return services, nil
}

func decodeRunAs(node *node, path []string) (RunAsSpec, error) {
	if node.kind != nodeMap {
		return RunAsSpec{}, errorAt(path, "expected mapping")
	}
	if err := rejectUnknownFields(node, path, []string{"user", "group"}); err != nil {
		return RunAsSpec{}, err
	}

	user, err := requireStringField(node, path, "user")
	if err != nil {
		return RunAsSpec{}, err
	}
	if strings.TrimSpace(user) == "" {
		return RunAsSpec{}, errorAt(append(path, "user"), "value must not be empty")
	}

	runAs := RunAsSpec{User: user}
	if groupNode, ok := node.mapEntries["group"]; ok {
		group, err := expectString(groupNode, append(path, "group"))
		if err != nil {
			return RunAsSpec{}, err
		}
		if strings.TrimSpace(group) == "" {
			return RunAsSpec{}, errorAt(append(path, "group"), "value must not be empty")
		}
		runAs.Group = group
	}

	return runAs, nil
}

func decodeSockets(node *node, path []string) (map[string]SocketSpec, error) {
	if node.kind != nodeMap {
		return nil, errorAt(path, "expected mapping")
	}

	sockets := make(map[string]SocketSpec, len(node.mapEntries))
	for _, socketPath := range sortedKeys(node.mapEntries) {
		if err := validateAbsolutePath(socketPath, path); err != nil {
			return nil, err
		}

		specPath := append(path, socketPath)
		specNode := node.mapEntries[socketPath]
		if specNode.kind != nodeMap {
			return nil, errorAt(specPath, "expected mapping")
		}
		if err := rejectUnknownFields(specNode, specPath, []string{"owner", "group", "mode"}); err != nil {
			return nil, err
		}

		spec := SocketSpec{}
		if ownerNode, ok := specNode.mapEntries["owner"]; ok {
			owner, err := expectNonEmptyString(ownerNode, append(specPath, "owner"))
			if err != nil {
				return nil, err
			}
			spec.Owner = owner
		}
		if groupNode, ok := specNode.mapEntries["group"]; ok {
			group, err := expectNonEmptyString(groupNode, append(specPath, "group"))
			if err != nil {
				return nil, err
			}
			spec.Group = group
		}
		if modeNode, ok := specNode.mapEntries["mode"]; ok {
			mode, err := expectString(modeNode, append(specPath, "mode"))
			if err != nil {
				return nil, err
			}
			if !isOctalMode(mode) {
				return nil, errorAt(append(specPath, "mode"), "invalid mode %q", mode)
			}
			spec.Mode = mode
		}

		sockets[socketPath] = spec
	}

	return sockets, nil
}

func decodePaths(node *node, path []string) (map[string]PathSpec, error) {
	if node.kind != nodeMap {
		return nil, errorAt(path, "expected mapping")
	}

	paths := make(map[string]PathSpec, len(node.mapEntries))
	for _, targetPath := range sortedKeys(node.mapEntries) {
		if err := validateAbsolutePath(targetPath, path); err != nil {
			return nil, err
		}

		specPath := append(path, targetPath)
		specNode := node.mapEntries[targetPath]
		if specNode.kind != nodeMap {
			return nil, errorAt(specPath, "expected mapping")
		}
		if err := rejectUnknownFields(specNode, specPath, []string{"owner", "group", "mode", "type"}); err != nil {
			return nil, err
		}

		spec := PathSpec{}
		if ownerNode, ok := specNode.mapEntries["owner"]; ok {
			owner, err := expectNonEmptyString(ownerNode, append(specPath, "owner"))
			if err != nil {
				return nil, err
			}
			spec.Owner = owner
		}
		if groupNode, ok := specNode.mapEntries["group"]; ok {
			group, err := expectNonEmptyString(groupNode, append(specPath, "group"))
			if err != nil {
				return nil, err
			}
			spec.Group = group
		}
		if modeNode, ok := specNode.mapEntries["mode"]; ok {
			mode, err := expectString(modeNode, append(specPath, "mode"))
			if err != nil {
				return nil, err
			}
			if !isOctalMode(mode) {
				return nil, errorAt(append(specPath, "mode"), "invalid mode %q", mode)
			}
			spec.Mode = mode
		}
		if typeNode, ok := specNode.mapEntries["type"]; ok {
			value, err := expectString(typeNode, append(specPath, "type"))
			if err != nil {
				return nil, err
			}
			if !ValidPathType(value) {
				return nil, errorAt(specPath, "invalid path type %q\n  valid values: file, directory", value)
			}
			spec.Type = PathType(value)
		}

		paths[targetPath] = spec
	}

	return paths, nil
}

func decodeIdentity(node *node, path []string) (map[string]IdentitySpec, error) {
	if node.kind != nodeMap {
		return nil, errorAt(path, "expected mapping")
	}

	identities := make(map[string]IdentitySpec, len(node.mapEntries))
	for _, name := range sortedKeys(node.mapEntries) {
		if strings.TrimSpace(name) == "" {
			return nil, errorAt(path, "identity name must not be empty")
		}

		specPath := append(path, name)
		specNode := node.mapEntries[name]
		if specNode.kind != nodeMap {
			return nil, errorAt(specPath, "expected mapping")
		}
		if err := rejectUnknownFields(specNode, specPath, []string{"service", "uid", "gid", "capabilities"}); err != nil {
			return nil, err
		}

		spec := IdentitySpec{}
		service, err := requireStringField(specNode, specPath, "service")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(service) == "" {
			return nil, errorAt(append(specPath, "service"), "value must not be empty")
		}
		spec.Service = service
		if uidNode, ok := specNode.mapEntries["uid"]; ok {
			uid, err := expectNonNegativeInt(uidNode, append(specPath, "uid"))
			if err != nil {
				return nil, err
			}
			spec.UID = &uid
		}
		if gidNode, ok := specNode.mapEntries["gid"]; ok {
			gid, err := expectNonNegativeInt(gidNode, append(specPath, "gid"))
			if err != nil {
				return nil, err
			}
			spec.GID = &gid
		}
		if capsNode, ok := specNode.mapEntries["capabilities"]; ok {
			caps, err := decodeCapabilitySetSpec(capsNode, append(specPath, "capabilities"))
			if err != nil {
				return nil, err
			}
			spec.Capabilities = &caps
		}

		if spec.UID == nil && spec.GID == nil && spec.Capabilities == nil {
			return nil, errorAt(specPath, "at least one of uid, gid, capabilities must be set")
		}

		identities[name] = spec
	}

	return identities, nil
}

func rejectUnknownFields(node *node, path []string, allowed []string) error {
	if node.kind != nodeMap {
		return errorAt(path, "expected mapping")
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}

	for _, field := range sortedKeys(node.mapEntries) {
		if _, ok := allowedSet[field]; ok {
			continue
		}

		hint := nearestField(field, allowed)
		location := formatPath(path)
		if hint == "" {
			return fmt.Errorf("unknown field %q at %s", field, location)
		}

		return fmt.Errorf("unknown field %q at %s\n  hint: did you mean %q?", field, location, hint)
	}

	return nil
}

func requireStringField(node *node, path []string, name string) (string, error) {
	child, ok := node.mapEntries[name]
	if !ok {
		return "", errorAt(append(path, name), "missing required field")
	}

	return expectString(child, append(path, name))
}

func requireMapField(node *node, path []string, name string) (*node, error) {
	child, ok := node.mapEntries[name]
	if !ok {
		return nil, errorAt(append(path, name), "missing required field")
	}
	if child.kind != nodeMap {
		return nil, errorAt(append(path, name), "expected mapping")
	}

	return child, nil
}

func expectString(node *node, path []string) (string, error) {
	if node.kind != nodeString {
		return "", errorAt(path, "expected string")
	}

	return node.stringVal, nil
}

func expectNonEmptyString(node *node, path []string) (string, error) {
	value, err := expectString(node, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", errorAt(path, "value must not be empty")
	}

	return value, nil
}

func expectNonNegativeInt(node *node, path []string) (int, error) {
	if node.kind != nodeInt {
		return 0, errorAt(path, "expected integer")
	}
	if node.intVal < 0 {
		return 0, errorAt(path, "value must be >= 0")
	}

	return node.intVal, nil
}

func decodeStringList(node *node, path []string) ([]string, error) {
	if node.kind != nodeList {
		return nil, errorAt(path, "expected list")
	}

	values := make([]string, 0, len(node.listItems))
	for index, item := range node.listItems {
		if item.kind != nodeString {
			return nil, errorAt(append(path, strconv.Itoa(index)), "expected string")
		}
		if strings.TrimSpace(item.stringVal) == "" {
			return nil, errorAt(append(path, strconv.Itoa(index)), "value must not be empty")
		}
		values = append(values, item.stringVal)
	}

	return values, nil
}

func decodeCapabilityList(node *node, path []string) ([]string, error) {
	values, err := decodeStringList(node, path)
	if err != nil {
		return nil, err
	}

	for index, value := range values {
		if capabilities.IsCanonical(value) {
			continue
		}

		candidate := capabilities.CanonicalizeToken(value)
		if candidate != "" && candidate != value && capabilities.IsCanonical(candidate) {
			return nil, errorAt(append(path, strconv.Itoa(index)), "invalid capability %q\n  hint: use canonical capability name %q", value, candidate)
		}

		return nil, errorAt(append(path, strconv.Itoa(index)), "invalid capability %q\n  valid form: CAP_FOO", value)
	}

	return values, nil
}

func decodeCapabilitySetSpec(node *node, path []string) (CapabilitySetSpec, error) {
	if node.kind != nodeMap {
		return CapabilitySetSpec{}, errorAt(path, "expected mapping")
	}
	if err := rejectUnknownFields(node, path, []string{"effective", "permitted", "inheritable", "bounding", "ambient"}); err != nil {
		return CapabilitySetSpec{}, err
	}

	spec := CapabilitySetSpec{}
	if child, ok := node.mapEntries["effective"]; ok {
		values, err := decodeCapabilityList(child, append(path, "effective"))
		if err != nil {
			return CapabilitySetSpec{}, err
		}
		spec.Effective = values
	}
	if child, ok := node.mapEntries["permitted"]; ok {
		values, err := decodeCapabilityList(child, append(path, "permitted"))
		if err != nil {
			return CapabilitySetSpec{}, err
		}
		spec.Permitted = values
	}
	if child, ok := node.mapEntries["inheritable"]; ok {
		values, err := decodeCapabilityList(child, append(path, "inheritable"))
		if err != nil {
			return CapabilitySetSpec{}, err
		}
		spec.Inheritable = values
	}
	if child, ok := node.mapEntries["bounding"]; ok {
		values, err := decodeCapabilityList(child, append(path, "bounding"))
		if err != nil {
			return CapabilitySetSpec{}, err
		}
		spec.Bounding = values
	}
	if child, ok := node.mapEntries["ambient"]; ok {
		values, err := decodeCapabilityList(child, append(path, "ambient"))
		if err != nil {
			return CapabilitySetSpec{}, err
		}
		spec.Ambient = values
	}

	if isCapabilitySetSpecEmpty(spec) {
		return CapabilitySetSpec{}, errorAt(path, "at least one capability set must be defined")
	}

	return spec, nil
}

func isCapabilitySetSpecEmpty(spec CapabilitySetSpec) bool {
	return spec.Effective == nil &&
		spec.Permitted == nil &&
		spec.Inheritable == nil &&
		spec.Bounding == nil &&
		spec.Ambient == nil
}

func validateAbsolutePath(value string, path []string) error {
	if strings.TrimSpace(value) == "" {
		return errorAt(path, "path must not be empty")
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("relative path %q at %s\n  hint: use an absolute path like %q", value, formatPath(path), "/"+strings.TrimLeft(value, "/"))
	}

	return nil
}

func isOctalMode(value string) bool {
	if len(value) != 4 && len(value) != 5 {
		return false
	}
	if value[0] != '0' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '7' {
			return false
		}
	}

	return true
}

func formatPath(path []string) string {
	if len(path) == 0 {
		return "root"
	}

	return strings.Join(path, ".")
}

func hasUnsupportedInlineComment(content string) bool {
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(content); i++ {
		ch := content[i]
		switch ch {
		case '\\':
			if inDouble {
				escaped = !escaped
			} else {
				escaped = false
			}
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			escaped = false
		case '"':
			if !inSingle && !escaped {
				inDouble = !inDouble
			} else {
				escaped = false
			}
		case '#':
			if !inSingle && !inDouble && i > 0 && content[i-1] == ' ' {
				return true
			}
			escaped = false
		default:
			escaped = false
		}
	}

	return false
}

func rejectUnsupportedScalarFeature(raw string, line int) error {
	switch {
	case strings.HasPrefix(raw, "&"):
		return fmt.Errorf("anchors are not supported at line %d", line)
	case strings.HasPrefix(raw, "*"):
		return fmt.Errorf("aliases are not supported at line %d", line)
	case strings.HasPrefix(raw, "!"):
		return fmt.Errorf("tags are not supported at line %d", line)
	default:
		return nil
	}
}

func rejectUnsupportedKeyFeature(key string, line int) error {
	switch {
	case key == "<<":
		return fmt.Errorf("merge keys are not supported at line %d", line)
	case strings.HasPrefix(key, "&"):
		return fmt.Errorf("anchors are not supported at line %d", line)
	case strings.HasPrefix(key, "*"):
		return fmt.Errorf("aliases are not supported at line %d", line)
	case strings.HasPrefix(key, "!"):
		return fmt.Errorf("tags are not supported at line %d", line)
	default:
		return nil
	}
}

func errorAt(path []string, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	location := formatPath(path)
	if index := strings.IndexByte(message, '\n'); index >= 0 {
		return fmt.Errorf("%s at %s%s", message[:index], location, message[index:])
	}

	return fmt.Errorf("%s at %s", message, location)
}

func sortedKeys(values map[string]*node) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nearestField(field string, allowed []string) string {
	best := ""
	bestDistance := -1
	for _, candidate := range allowed {
		distance := levenshtein(field, candidate)
		if bestDistance == -1 || distance < bestDistance {
			bestDistance = distance
			best = candidate
		}
	}
	if bestDistance > 3 {
		return ""
	}

	return best
}

func levenshtein(left, right string) int {
	if left == right {
		return 0
	}
	if len(left) == 0 {
		return len(right)
	}
	if len(right) == 0 {
		return len(left)
	}

	column := make([]int, len(right)+1)
	for i := range column {
		column[i] = i
	}

	for i := 1; i <= len(left); i++ {
		previous := column[0]
		column[0] = i
		for j := 1; j <= len(right); j++ {
			old := column[j]
			cost := 0
			if left[i-1] != right[j-1] {
				cost = 1
			}

			insertion := column[j] + 1
			deletion := column[j-1] + 1
			substitution := previous + cost
			column[j] = minInt(insertion, deletion, substitution)
			previous = old
		}
	}

	return column[len(right)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}
