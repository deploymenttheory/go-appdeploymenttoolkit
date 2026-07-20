package manifest

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the semantic version of the generated JSON Schema
// artifacts. The manifest apiVersion ("v" + SchemaVersion) and every
// per-platform schema artifact share this one constant.
const SchemaVersion = "0.1.0-alpha"

// SchemaFileName is the file name of the Windows schema artifact.
const SchemaFileName = "winadt.schema-v" + SchemaVersion + ".json"

// SchemaFileNameFor returns the per-platform schema artifact file name
// (winadt.schema-v<semver>.json / macadt.schema-v<semver>.json).
func SchemaFileNameFor(target Platform) string {
	prefix := "winadt"
	if target == PlatformDarwin {
		prefix = "macadt"
	}
	return prefix + ".schema-v" + SchemaVersion + ".json"
}

// schemaIDBase is the canonical identifier prefix of the generated schemas.
const schemaIDBase = "https://github.com/deploymenttheory/go-appdeploymenttoolkit/manifest/"

// SchemaID is the canonical identifier of the Windows JSON Schema.
const SchemaID = schemaIDBase + SchemaFileName

// JSONSchema renders the manifest format as a JSON Schema (draft 2020-12)
// generated from the live step registry and session table, so the recorded
// schema can never drift from what the validator accepts (a test compares it
// against the checked-in adt-v1alpha1.schema.json).
//
// The schema captures structure, types, required parameters and enums; the
// cross-field semantic rules (StepSpec.Check) and the package-file existence
// and platform layers exist only in the Go validator, which remains
// authoritative. Enum values are recorded in canonical casing; adt itself
// matches them case-insensitively.
//
// Point an editor at it for autocomplete and inline validation:
//
//	# yaml-language-server: $schema=./winadt.schema-v0.1.0-alpha.json
//
// (emit the file beside a package with `adt schema > <SchemaFileName>`).
//
// The target platform selects the step subset: only steps whose Platforms
// include target appear in the schema. A target with no registered steps is
// an error.
func JSONSchema(target Platform) ([]byte, error) {
	steps := stepsFor(target)
	if len(steps) == 0 {
		return nil, fmt.Errorf("manifest: no steps registered for target %q", target)
	}
	schema := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         schemaIDBase + SchemaFileNameFor(target),
		"title":       "adt deployment manifest (" + APIVersion + ", " + string(target) + ")",
		"description": "A go-appdeploymenttoolkit deployment: session metadata plus nine phase slots composed of steps from the adt step catalog. Structural schema only — `adt validate` additionally applies cross-field semantic rules, package-file existence checks and platform gating.",
		"type":        "object",
		"additionalProperties": false,
		"required":             []string{"apiVersion", "kind", "session", "phases"},
		"properties": map[string]any{
			"apiVersion": map[string]any{
				"description": "Manifest schema version.",
				"const":       APIVersion,
			},
			"kind": map[string]any{
				"description": "Manifest kind.",
				"const":       KindDeployment,
			},
			"session": sessionSchema(),
			"phases":  phasesSchema(),
		},
		"$defs": map[string]any{
			"step":        stepEnvelopeSchema(steps),
			"processList": processListSchema(),
			"duration": map[string]any{
				"type":        "string",
				"description": `Go duration string, e.g. "90s", "5m", "1h30m". Bare numbers are rejected.`,
				"pattern":     `^-?([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`,
			},
			"timestamp": map[string]any{
				"type":        "string",
				"description": `RFC 3339 timestamp ("2026-08-01T17:00:00Z") or bare date ("2026-08-01", local midnight).`,
				"anyOf": []any{
					map[string]any{"format": "date-time"},
					map[string]any{"pattern": `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`},
				},
			},
		},
	}
	return json.MarshalIndent(schema, "", "  ")
}

// sessionSchema renders the session block from sessionParamSpecs.
func sessionSchema() map[string]any {
	props, required := paramProperties(sessionParamSpecs)
	s := map[string]any{
		"description":          "Deployment session metadata (curated mirror of adt.SessionOptions).",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// phasesSchema renders the nine phase slots.
func phasesSchema() map[string]any {
	slots := map[string]any{}
	for _, name := range PhaseNames {
		slots[name] = map[string]any{
			"description": "Steps of the " + name + " phase, run in order.",
			"oneOf": []any{
				map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/step"}},
				map[string]any{"type": "null"},
			},
		}
	}
	return map[string]any{
		"description":          "The nine phase slots; all optional, each a list of steps.",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           slots,
	}
}

// stepsFor returns the catalog subset supporting the target platform.
func stepsFor(target Platform) []StepSpec {
	var out []StepSpec
	for _, s := range Steps() {
		if s.SupportsPlatform(target) {
			out = append(out, s)
		}
	}
	return out
}

// stepEnvelopeSchema renders the step envelope plus a per-step conditional
// binding `uses` to its `with` parameter schema.
func stepEnvelopeSchema(steps []StepSpec) map[string]any {
	usesEnum := make([]any, len(steps))
	conditions := make([]any, 0, len(steps))
	for i, spec := range steps {
		usesEnum[i] = spec.Name
		props, required := paramProperties(spec.Params)
		withSchema := map[string]any{
			"description":          spec.Summary,
			"type":                 "object",
			"additionalProperties": false,
			"properties":           props,
		}
		withProperties := map[string]any{"with": withSchema}
		stepThen := map[string]any{"properties": withProperties}
		if len(required) > 0 {
			withSchema["required"] = required
			// A step with required params must carry a `with` block.
			stepThen["required"] = []string{"with"}
		}
		conditions = append(conditions, map[string]any{
			"if": map[string]any{
				"properties": map[string]any{"uses": map[string]any{"const": spec.Name}},
				"required":   []string{"uses"},
			},
			"then": stepThen,
		})
	}
	return map[string]any{
		"description":          "One workflow step invocation.",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"uses"},
		"properties": map[string]any{
			"uses": map[string]any{
				"description": "Step catalog entry (see `adt steps`).",
				"enum":        usesEnum,
			},
			"name":            map[string]any{"type": "string", "description": "Optional display name used in logs."},
			"continueOnError": map[string]any{"type": "boolean", "description": "Log a step failure and keep the phase going."},
			"with":            map[string]any{"type": "object", "description": "Step parameters (schema depends on `uses`)."},
		},
		"allOf": conditions,
	}
}

// processListSchema renders the closeProcesses entry shape.
func processListSchema() map[string]any {
	props, required := paramProperties(processListSpecs)
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           props,
		},
	}
}

// paramProperties renders a ParamSpec table as JSON Schema properties plus
// the required-name list.
func paramProperties(specs []ParamSpec) (map[string]any, []string) {
	props := map[string]any{}
	var required []string
	for _, spec := range specs {
		props[spec.Name] = paramSchema(spec)
		if spec.Required {
			required = append(required, spec.Name)
		}
	}
	return props, required
}

// paramSchema renders one ParamSpec.
func paramSchema(spec ParamSpec) map[string]any {
	s := map[string]any{"description": spec.Description}
	switch spec.Type {
	case TypeString:
		s["type"] = "string"
		if len(spec.Enum) > 0 {
			enum := make([]any, len(spec.Enum))
			for i, v := range spec.Enum {
				enum[i] = v
			}
			s["enum"] = enum
			s["description"] = spec.Description + " (matched case-insensitively by adt)"
		}
	case TypeBool:
		s["type"] = "boolean"
	case TypeInt:
		s["type"] = "integer"
	case TypeFloat:
		s["type"] = "number"
	case TypeDuration:
		s = map[string]any{"$ref": "#/$defs/duration", "description": spec.Description}
	case TypeTimestamp:
		s = map[string]any{"$ref": "#/$defs/timestamp", "description": spec.Description}
	case TypeStringList:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "string"}
	case TypeIntList:
		s["type"] = "array"
		s["items"] = map[string]any{"type": "integer"}
	case TypeProcessList:
		s = map[string]any{"$ref": "#/$defs/processList", "description": spec.Description}
	case TypeAny:
		s["oneOf"] = []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "integer"},
			map[string]any{"type": "number"},
			map[string]any{"type": "boolean"},
			map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}
	default:
		panic(fmt.Sprintf("manifest: unhandled ParamType %q in schema generation", spec.Type))
	}
	if spec.Default != nil {
		s["default"] = spec.Default
	}
	return s
}
