package manifest

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// APIVersion is the manifest schema version this build understands: the
// semver [SchemaVersion] with a "v" prefix. The manifest's apiVersion, the
// schema artifact's filename and its $id all derive from the one constant.
const APIVersion = "v" + SchemaVersion

// KindDeployment is the only manifest kind in v1alpha1.
const KindDeployment = "Deployment"

// PhaseNames are the nine phase slots in execution-relevant order.
var PhaseNames = []string{
	"preInstall", "install", "postInstall",
	"preUninstall", "uninstall", "postUninstall",
	"preRepair", "repair", "postRepair",
}

// Manifest is the parsed deployment manifest.
type Manifest struct {
	// Path is the source file (as passed to Load).
	Path       string
	APIVersion string
	Kind       string
	// Session is the decoded session: block (schema of steps_config.go).
	Session Params
	// Phases holds all nine slots in PhaseNames order (empty slices for
	// absent phases).
	Phases []Phase
}

// Phase is one named phase slot.
type Phase struct {
	Name  string
	Steps []Step
}

// PhaseSteps returns the steps of the named phase.
func (m *Manifest) PhaseSteps(name string) []Step {
	for _, p := range m.Phases {
		if p.Name == name {
			return p.Steps
		}
	}
	return nil
}

// Step is one workflow step invocation.
type Step struct {
	// Uses is the registry step name (e.g. "msi.install").
	Uses string
	// DisplayName is the optional author-facing name.
	DisplayName string
	// ContinueOnError logs a step failure and keeps the phase going.
	ContinueOnError bool
	// With is the decoded parameter block (schema-valid subset).
	With Params
	// Pos is the step mapping's position; UsesPos the `uses:` value's.
	Pos     Pos
	UsesPos Pos
}

// stepEnvelopeKeys are the only keys allowed on a step mapping.
var stepEnvelopeKeys = []string{"uses", "with", "name", "continueOnError"}

// Load reads and schema-validates a manifest file (validation layer a). A
// hard failure (unreadable file, YAML syntax error) returns err; recoverable
// problems are reported as issues alongside a best-effort Manifest so the
// remaining validation layers can still run over the valid parts.
func Load(path string) (*Manifest, []Issue, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- validating the caller-designated manifest
	if err != nil {
		return nil, nil, fmt.Errorf("manifest: reading %s: %w", path, err)
	}
	return Parse(path, data)
}

// Parse is Load over in-memory bytes (path is used for reporting only).
func Parse(path string, data []byte) (*Manifest, []Issue, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("manifest: parsing %s: %w", path, err)
	}
	var issues []Issue
	m := &Manifest{Path: path}
	for _, name := range PhaseNames {
		m.Phases = append(m.Phases, Phase{Name: name})
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		addIssue(&issues, SeverityError, CodeBadType, "", Pos{Line: 1, Col: 1}, "empty manifest")
		return m, issues, nil
	}
	doc := resolveAlias(root.Content[0])
	if doc.Kind != yaml.MappingNode {
		addIssue(&issues, SeverityError, CodeBadType, "", nodePos(doc),
			"manifest root must be a mapping")
		return m, issues, nil
	}

	var sessionNode, phasesNode *yaml.Node
	seen := map[string]bool{}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode, valNode := doc.Content[i], resolveAlias(doc.Content[i+1])
		key := keyNode.Value
		if seen[key] {
			addIssue(&issues, SeverityError, CodeDuplicateKey, key, nodePos(keyNode),
				"duplicate key %q", key)
			continue
		}
		seen[key] = true
		switch key {
		case "apiVersion":
			m.APIVersion = valNode.Value
			if valNode.Value != APIVersion {
				addIssue(&issues, SeverityError, CodeBadEnum, "apiVersion", nodePos(valNode),
					"unsupported apiVersion %q (this build understands %q)", valNode.Value, APIVersion)
			}
		case "kind":
			m.Kind = valNode.Value
			if valNode.Value != KindDeployment {
				addIssue(&issues, SeverityError, CodeBadEnum, "kind", nodePos(valNode),
					"unsupported kind %q (expected %q)", valNode.Value, KindDeployment)
			}
		case "session":
			sessionNode = valNode
		case "phases":
			phasesNode = valNode
		default:
			addIssue(&issues, SeverityError, CodeUnknownField, key, nodePos(keyNode),
				"unknown top-level key %q (expected apiVersion, kind, session, phases)", key)
		}
	}
	if !seen["apiVersion"] {
		addIssue(&issues, SeverityError, CodeMissingRequired, "apiVersion", nodePos(doc),
			"apiVersion is required")
	}
	if !seen["kind"] {
		addIssue(&issues, SeverityError, CodeMissingRequired, "kind", nodePos(doc),
			"kind is required")
	}
	if sessionNode == nil {
		addIssue(&issues, SeverityError, CodeMissingRequired, "session", nodePos(doc),
			"session block is required")
	} else {
		m.Session = walkMapping(sessionNode, "session", sessionParamSpecs, nodePos(sessionNode), &issues)
	}
	if phasesNode == nil {
		addIssue(&issues, SeverityError, CodeMissingRequired, "phases", nodePos(doc),
			"phases block is required")
	} else {
		parsePhases(m, phasesNode, &issues)
	}
	return m, issues, nil
}

// parsePhases walks the phases: mapping into the manifest's fixed slots.
func parsePhases(m *Manifest, node *yaml.Node, issues *[]Issue) {
	if node.Kind != yaml.MappingNode {
		addIssue(issues, SeverityError, CodeBadType, "phases", nodePos(node),
			"phases must be a mapping of phase name to step list")
		return
	}
	slot := map[string]int{}
	for i, p := range m.Phases {
		slot[p.Name] = i
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valNode := node.Content[i], resolveAlias(node.Content[i+1])
		name := keyNode.Value
		path := "phases." + name
		if seen[name] {
			addIssue(issues, SeverityError, CodeDuplicateKey, path, nodePos(keyNode),
				"duplicate phase %q", name)
			continue
		}
		seen[name] = true
		idx, known := slot[name]
		if !known {
			suggestion := ""
			if best := closestName(name, PhaseNames); best != "" {
				suggestion = fmt.Sprintf(" — did you mean %q?", best)
			}
			addIssue(issues, SeverityError, CodeUnknownField, path, nodePos(keyNode),
				"unknown phase %q (valid phases: %s)%s", name, strings.Join(PhaseNames, ", "), suggestion)
			continue
		}
		if isNull(valNode) {
			continue // `phase:` with no steps is an explicit empty phase
		}
		if valNode.Kind != yaml.SequenceNode {
			addIssue(issues, SeverityError, CodeBadType, path, nodePos(valNode),
				"phase %q must be a list of steps", name)
			continue
		}
		for j, stepNode := range valNode.Content {
			stepPath := path + "[" + strconv.Itoa(j) + "]"
			if step, ok := parseStep(resolveAlias(stepNode), stepPath, issues); ok {
				m.Phases[idx].Steps = append(m.Phases[idx].Steps, step)
			}
		}
	}
}

// parseStep decodes one step envelope and its `with:` block against the
// registry's ParamSpec table.
func parseStep(node *yaml.Node, path string, issues *[]Issue) (Step, bool) {
	if node.Kind != yaml.MappingNode {
		addIssue(issues, SeverityError, CodeBadType, path, nodePos(node),
			"a step must be a mapping with a `uses` key")
		return Step{}, false
	}
	step := Step{Pos: nodePos(node)}
	var withNode *yaml.Node
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
		switch key {
		case "uses":
			step.Uses = valNode.Value
			step.UsesPos = nodePos(valNode)
		case "name":
			step.DisplayName = valNode.Value
		case "continueOnError":
			if v, ok := asBool(valNode, keyPath, issues); ok {
				step.ContinueOnError = v.(bool)
			}
		case "with":
			withNode = valNode
		default:
			addIssue(issues, SeverityError, CodeUnknownField, keyPath, nodePos(keyNode),
				"unknown step key %q (expected %s)%s",
				key, strings.Join(stepEnvelopeKeys, ", "), didYouMeanStepKey(key))
		}
	}
	if step.Uses == "" {
		addIssue(issues, SeverityError, CodeMissingRequired, path+".uses", step.Pos,
			"step is missing the required `uses` key")
		return Step{}, false
	}
	spec, known := Lookup(step.Uses)
	if !known {
		names := make([]string, 0, len(catalog))
		for name := range catalog {
			names = append(names, name)
		}
		suggestion := ""
		if best := closestName(step.Uses, names); best != "" {
			suggestion = fmt.Sprintf(" — did you mean %q?", best)
		}
		addIssue(issues, SeverityError, CodeUnknownStep, path+".uses", step.UsesPos,
			"unknown step %q%s (run `adt steps` for the catalog)", step.Uses, suggestion)
		return Step{}, false
	}
	step.With = walkMapping(withNode, path+".with", spec.Params, step.Pos, issues)
	return step, true
}

// didYouMeanStepKey suggests a close step-envelope key.
func didYouMeanStepKey(key string) string {
	if best := closestName(key, stepEnvelopeKeys); best != "" {
		return fmt.Sprintf(" — did you mean %q?", best)
	}
	return ""
}
