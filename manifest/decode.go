package manifest

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
)

// nodePos extracts a node's source position.
func nodePos(n *yaml.Node) Pos { return Pos{Line: n.Line, Col: n.Column} }

// addIssue appends an issue.
func addIssue(issues *[]Issue, sev Severity, code, path string, pos Pos, format string, args ...any) {
	*issues = append(*issues, Issue{
		Severity: sev,
		Code:     code,
		Path:     path,
		Pos:      pos,
		Message:  fmt.Sprintf(format, args...),
	})
}

// resolveAlias follows alias nodes to their anchor target.
func resolveAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// isNull reports a YAML null scalar.
func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || (n.Tag == "" && n.Value == ""))
}

// walkMapping decodes a YAML mapping against a ParamSpec table: unknown and
// duplicate keys are flagged, known keys are coerced per their declared type
// with positions retained, and missing required params are reported at the
// mapping itself (or fallbackPos when the mapping is absent). A nil node
// (absent optional mapping) yields empty Params.
func walkMapping(node *yaml.Node, path string, specs []ParamSpec, fallbackPos Pos, issues *[]Issue) Params {
	p := Params{}
	byName := make(map[string]ParamSpec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
	}
	mappingPos := fallbackPos
	if node != nil {
		node = resolveAlias(node)
		mappingPos = nodePos(node)
		if node.Kind != yaml.MappingNode {
			addIssue(issues, SeverityError, CodeBadType, path, mappingPos, "expected a mapping")
			return p
		}
		seen := map[string]bool{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode, valNode := node.Content[i], resolveAlias(node.Content[i+1])
			key := keyNode.Value
			keyPath := path + "." + key
			if seen[key] {
				addIssue(issues, SeverityError, CodeDuplicateKey, keyPath, nodePos(keyNode),
					"duplicate key %q", key)
				continue
			}
			seen[key] = true
			spec, known := byName[key]
			if !known {
				addIssue(issues, SeverityError, CodeUnknownField, keyPath, nodePos(keyNode),
					"unknown field %q%s", key, didYouMeanParam(key, specs))
				continue
			}
			if v, ok := coerce(valNode, keyPath, spec, issues); ok {
				p.set(key, Value{V: v, Pos: nodePos(valNode)})
			}
		}
	}
	for _, s := range specs {
		if s.Required && !p.Has(s.Name) {
			addIssue(issues, SeverityError, CodeMissingRequired, path+"."+s.Name, mappingPos,
				"required parameter %q is missing", s.Name)
		}
	}
	return p
}

// coerce converts a value node per the spec's ParamType, reporting issues.
func coerce(n *yaml.Node, path string, spec ParamSpec, issues *[]Issue) (any, bool) {
	if isNull(n) {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"null is not a valid %s", spec.Type)
		return nil, false
	}
	switch spec.Type {
	case TypeString:
		return asString(n, path, spec, issues)
	case TypeBool:
		return asBool(n, path, issues)
	case TypeInt:
		return asInt(n, path, issues)
	case TypeFloat:
		return asFloat(n, path, issues)
	case TypeDuration:
		return asDuration(n, path, issues)
	case TypeTimestamp:
		return asTimestamp(n, path, issues)
	case TypeStringList:
		return asStringList(n, path, issues)
	case TypeIntList:
		return asIntList(n, path, issues)
	case TypeProcessList:
		return asProcessList(n, path, issues)
	case TypeAny:
		return asAny(n, path, issues)
	default:
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"internal: unhandled parameter type %q", spec.Type)
		return nil, false
	}
}

func asString(n *yaml.Node, path string, spec ParamSpec, issues *[]Issue) (any, bool) {
	if n.Kind != yaml.ScalarNode {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n), "expected a string")
		return nil, false
	}
	v := n.Value
	if len(spec.Enum) > 0 && !enumContains(spec.Enum, v) {
		addIssue(issues, SeverityError, CodeBadEnum, path, nodePos(n),
			"%q is not one of %s", v, strings.Join(spec.Enum, ", "))
		return nil, false
	}
	return v, true
}

func asBool(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind == yaml.ScalarNode {
		if b, err := strconv.ParseBool(strings.ToLower(n.Value)); err == nil {
			return b, true
		}
	}
	addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
		"expected true or false, got %q", n.Value)
	return nil, false
}

func asInt(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind == yaml.ScalarNode {
		if i, err := strconv.Atoi(n.Value); err == nil {
			return i, true
		}
	}
	addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
		"expected an integer, got %q", n.Value)
	return nil, false
}

func asFloat(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind == yaml.ScalarNode {
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
			return f, true
		}
	}
	addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
		"expected a number, got %q", n.Value)
	return nil, false
}

func asDuration(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind != yaml.ScalarNode {
		addIssue(issues, SeverityError, CodeBadDuration, path, nodePos(n),
			`expected a duration string such as "90s" or "1h30m"`)
		return nil, false
	}
	if _, err := strconv.Atoi(n.Value); err == nil {
		addIssue(issues, SeverityError, CodeBadDuration, path, nodePos(n),
			`bare number %q is not a duration — did you mean %q?`, n.Value, n.Value+"s")
		return nil, false
	}
	d, err := time.ParseDuration(n.Value)
	if err != nil {
		addIssue(issues, SeverityError, CodeBadDuration, path, nodePos(n),
			`%q is not a valid duration (use Go duration syntax such as "90s", "5m", "1h30m")`, n.Value)
		return nil, false
	}
	return d, true
}

func asTimestamp(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind == yaml.ScalarNode {
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, n.Value, time.Local); err == nil {
				return t, true
			}
		}
	}
	addIssue(issues, SeverityError, CodeBadTimestamp, path, nodePos(n),
		`%q is not a valid timestamp (use RFC 3339 like "2026-08-01T17:00:00Z" or a date like "2026-08-01")`,
		n.Value)
	return nil, false
}

func asStringList(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind != yaml.SequenceNode {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"expected a list of strings")
		return nil, false
	}
	out := make([]string, 0, len(n.Content))
	ok := true
	for i, item := range n.Content {
		item = resolveAlias(item)
		if item.Kind != yaml.ScalarNode || isNull(item) {
			addIssue(issues, SeverityError, CodeBadType, fmt.Sprintf("%s[%d]", path, i),
				nodePos(item), "expected a string")
			ok = false
			continue
		}
		out = append(out, item.Value)
	}
	return out, ok
}

func asIntList(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind != yaml.SequenceNode {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"expected a list of integers")
		return nil, false
	}
	out := make([]int, 0, len(n.Content))
	ok := true
	for i, item := range n.Content {
		item = resolveAlias(item)
		v, err := strconv.Atoi(item.Value)
		if item.Kind != yaml.ScalarNode || err != nil {
			addIssue(issues, SeverityError, CodeBadType, fmt.Sprintf("%s[%d]", path, i),
				nodePos(item), "expected an integer, got %q", item.Value)
			ok = false
			continue
		}
		out = append(out, v)
	}
	return out, ok
}

// processListSpecs is the schema of one closeProcesses entry.
var processListSpecs = []ParamSpec{
	{Name: "name", Type: TypeString, Required: true, Description: "process name without extension"},
	{Name: "description", Type: TypeString, Description: "friendly name shown in dialogs"},
}

func asProcessList(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	if n.Kind != yaml.SequenceNode {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"expected a list of {name, description?} mappings")
		return nil, false
	}
	out := make([]deploy.ProcessObject, 0, len(n.Content))
	before := len(*issues)
	for i, item := range n.Content {
		entry := walkMapping(resolveAlias(item), fmt.Sprintf("%s[%d]", path, i), processListSpecs, nodePos(item), issues)
		name, _ := entry.String("name")
		out = append(out, deploy.ProcessObject{
			Name:        name,
			Description: entry.StringOr("description", ""),
		})
	}
	return out, len(*issues) == before
}

// asAny accepts scalars and string sequences, mapping YAML typing onto Go
// values (used only by registry.set's value param).
func asAny(n *yaml.Node, path string, issues *[]Issue) (any, bool) {
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int":
			if i, err := strconv.Atoi(n.Value); err == nil {
				return i, true
			}
		case "!!bool":
			if b, err := strconv.ParseBool(strings.ToLower(n.Value)); err == nil {
				return b, true
			}
		case "!!float":
			if f, err := strconv.ParseFloat(n.Value, 64); err == nil {
				return f, true
			}
		}
		return n.Value, true
	case yaml.SequenceNode:
		return asStringList(n, path, issues)
	default:
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(n),
			"expected a scalar or a list of strings")
		return nil, false
	}
}

// enumContains matches case-insensitively against the canonical enum list.
func enumContains(enum []string, v string) bool {
	for _, e := range enum {
		if strings.EqualFold(e, v) {
			return true
		}
	}
	return false
}

// didYouMeanParam suggests the closest known parameter name.
func didYouMeanParam(key string, specs []ParamSpec) string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	if best := closestName(key, names); best != "" {
		return fmt.Sprintf(" — did you mean %q?", best)
	}
	return ""
}

// closestName returns the candidate within edit distance <=2 of key ("" when
// nothing is close enough).
func closestName(key string, candidates []string) string {
	best, bestDist := "", 3
	for _, c := range candidates {
		if d := editDistance(strings.ToLower(key), strings.ToLower(c)); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// editDistance is a plain Levenshtein distance.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
