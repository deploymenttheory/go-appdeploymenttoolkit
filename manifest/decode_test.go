package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// parseNode unmarshals inline YAML into its root content node.
func parseNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(src), &root))
	require.NotEmpty(t, root.Content)
	return root.Content[0]
}

func TestWalkMappingCoercers(t *testing.T) {
	specs := []ParamSpec{
		{Name: "s", Type: TypeString},
		{Name: "e", Type: TypeString, Enum: []string{"alpha", "beta"}},
		{Name: "b", Type: TypeBool},
		{Name: "i", Type: TypeInt},
		{Name: "f", Type: TypeFloat},
		{Name: "d", Type: TypeDuration},
		{Name: "ts", Type: TypeTimestamp},
		{Name: "sl", Type: TypeStringList},
		{Name: "il", Type: TypeIntList},
		{Name: "pl", Type: TypeProcessList},
	}
	src := `
s: hello
e: Beta
b: true
i: 42
f: 3.5
d: 1h30m
ts: "2026-08-01T17:00:00Z"
sl: [a, b]
il: [1, 2]
pl:
  - name: widget
    description: Widget
`
	var issues []Issue
	p := walkMapping(parseNode(t, src), "with", specs, Pos{}, &issues)
	require.Empty(t, issues)

	s, _ := p.String("s")
	assert.Equal(t, "hello", s)
	e, _ := p.String("e")
	assert.Equal(t, "Beta", e, "enums match case-insensitively but keep the written value")
	b, _ := p.Bool("b")
	assert.True(t, b)
	i, _ := p.Int("i")
	assert.Equal(t, 42, i)
	f, _ := p.Float("f")
	assert.InDelta(t, 3.5, f, 0.001)
	d, _ := p.Duration("d")
	assert.Equal(t, 90*time.Minute, d)
	ts, _ := p.Time("ts")
	assert.Equal(t, 2026, ts.Year())
	sl, _ := p.StringList("sl")
	assert.Equal(t, []string{"a", "b"}, sl)
	il, _ := p.IntList("il")
	assert.Equal(t, []int{1, 2}, il)
	pl, _ := p.ProcessList("pl")
	require.Len(t, pl, 1)
	assert.Equal(t, "widget", pl[0].Name)

	pos, ok := p.PosOf("s")
	require.True(t, ok)
	assert.Equal(t, 2, pos.Line, "positions are retained")
}

func TestWalkMappingErrors(t *testing.T) {
	specs := []ParamSpec{
		{Name: "s", Type: TypeString, Required: true},
		{Name: "e", Type: TypeString, Enum: []string{"alpha", "beta"}},
		{Name: "d", Type: TypeDuration},
		{Name: "ts", Type: TypeTimestamp},
		{Name: "il", Type: TypeIntList},
	}
	cases := []struct {
		name string
		src  string
		code string
		line int
	}{
		{"unknown key", "s: x\nz: 1\n", CodeUnknownField, 2},
		{"missing required", "e: alpha\n", CodeMissingRequired, 1},
		{"bad enum", "s: x\ne: gamma\n", CodeBadEnum, 2},
		{"bare number duration", "s: x\nd: 90\n", CodeBadDuration, 2},
		{"garbage duration", "s: x\nd: soon\n", CodeBadDuration, 2},
		{"bad timestamp", "s: x\nts: tomorrow\n", CodeBadTimestamp, 2},
		{"bad int list", "s: x\nil: [1, two]\n", CodeBadType, 2},
		{"list for scalar", "s: [a]\n", CodeBadType, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var issues []Issue
			walkMapping(parseNode(t, tc.src), "with", specs, Pos{}, &issues)
			require.NotEmpty(t, issues)
			found := false
			for _, i := range issues {
				if i.Code == tc.code {
					found = true
					if tc.line > 0 {
						assert.Equal(t, tc.line, i.Pos.Line, "issue line for %s", tc.name)
					}
				}
			}
			assert.True(t, found, "expected code %s, got %+v", tc.code, issues)
		})
	}
}

func TestWalkMappingDuplicateKey(t *testing.T) {
	// yaml.v3 tolerates duplicate mapping keys at the node level.
	src := "s: one\ns: two\n"
	var issues []Issue
	walkMapping(parseNode(t, src), "with", []ParamSpec{{Name: "s", Type: TypeString}}, Pos{}, &issues)
	require.NotEmpty(t, issues)
	assert.Equal(t, CodeDuplicateKey, issues[0].Code)
}

func TestDidYouMean(t *testing.T) {
	specs := []ParamSpec{{Name: "deferTimes", Type: TypeInt}}
	var issues []Issue
	walkMapping(parseNode(t, "deferTimez: 1\n"), "with", specs, Pos{}, &issues)
	require.NotEmpty(t, issues)
	assert.Contains(t, issues[0].Message, `did you mean "deferTimes"`)
}
