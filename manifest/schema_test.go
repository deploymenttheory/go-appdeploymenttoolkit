package manifest

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaFile is the checked-in generated schema.
const schemaFile = SchemaFileName

// TestSchemaFileCurrent keeps the checked-in schema in lockstep with the
// registry: regenerate with
//
//	ADT_UPDATE_SCHEMA=1 go test ./manifest -run TestSchemaFileCurrent
func TestSchemaFileCurrent(t *testing.T) {
	generated, err := JSONSchema()
	require.NoError(t, err)
	generated = append(generated, '\n')

	if os.Getenv("ADT_UPDATE_SCHEMA") != "" {
		require.NoError(t, os.WriteFile(schemaFile, generated, 0o644))
	}
	onDisk, err := os.ReadFile(schemaFile)
	require.NoError(t, err, "checked-in schema missing; run: ADT_UPDATE_SCHEMA=1 go test ./manifest -run TestSchemaFileCurrent")
	assert.Equal(t, string(normalizeEOL(generated)), string(normalizeEOL(onDisk)),
		"schema drifted from the registry; regenerate with ADT_UPDATE_SCHEMA=1")
}

// normalizeEOL strips CR so the comparison survives git eol translation.
func normalizeEOL(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != '\r' {
			out = append(out, c)
		}
	}
	return out
}

func TestSchemaStructure(t *testing.T) {
	blob, err := JSONSchema()
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(blob, &schema))

	assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema["$schema"])
	assert.Equal(t, SchemaID, schema["$id"])

	props := schema["properties"].(map[string]any)
	assert.Equal(t, APIVersion, props["apiVersion"].(map[string]any)["const"])

	// Every phase slot is present.
	phaseProps := props["phases"].(map[string]any)["properties"].(map[string]any)
	for _, name := range PhaseNames {
		assert.Contains(t, phaseProps, name)
	}

	// The step envelope enumerates the full catalog and conditions each step.
	step := schema["$defs"].(map[string]any)["step"].(map[string]any)
	usesEnum := step["properties"].(map[string]any)["uses"].(map[string]any)["enum"].([]any)
	assert.Len(t, usesEnum, len(Steps()))
	assert.Len(t, step["allOf"].([]any), len(Steps()))

	// Spot-check one step's with-schema: msi.install requires path.
	for _, cond := range step["allOf"].([]any) {
		c := cond.(map[string]any)
		ifUses := c["if"].(map[string]any)["properties"].(map[string]any)["uses"].(map[string]any)["const"]
		if ifUses != "msi.install" {
			continue
		}
		then := c["then"].(map[string]any)
		assert.Equal(t, []any{"with"}, then["required"])
		with := then["properties"].(map[string]any)["with"].(map[string]any)
		assert.Equal(t, []any{"path"}, with["required"])
		withProps := with["properties"].(map[string]any)
		assert.Contains(t, withProps, "transforms")
		return
	}
	t.Fatal("msi.install condition not found in schema")
}
