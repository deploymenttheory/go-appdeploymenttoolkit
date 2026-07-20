package manifest

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/procmgmt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/shortcut"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/svcmgmt"
)

var stepNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)+$`)

func TestRegistryInvariants(t *testing.T) {
	steps := Steps()
	require.NotEmpty(t, steps)
	seen := map[string]bool{}
	for _, s := range steps {
		assert.False(t, seen[s.Name], "duplicate step %s", s.Name)
		seen[s.Name] = true
		assert.Regexp(t, stepNameRe, s.Name)
		assert.NotEmpty(t, s.Summary, "%s needs a summary", s.Name)
		assert.NotEmpty(t, s.Platforms, "%s needs platform tags", s.Name)
		assert.NotNil(t, s.Bind, "%s needs a Bind", s.Name)
		paramSeen := map[string]bool{}
		for _, p := range s.Params {
			assert.False(t, paramSeen[p.Name], "%s duplicate param %s", s.Name, p.Name)
			paramSeen[p.Name] = true
			assert.NotEmpty(t, p.Type, "%s.%s needs a type", s.Name, p.Name)
			assert.NotEmpty(t, p.Description, "%s.%s needs a description", s.Name, p.Name)
		}
	}
}

// TestEnumDrift asserts every enum value in the catalog round-trips through
// the parser that will consume it at runtime, so validation and execution can
// never disagree.
func TestEnumDrift(t *testing.T) {
	mustFind := func(stepName, param string) ParamSpec {
		s, ok := Lookup(stepName)
		require.True(t, ok, stepName)
		p, ok := s.Param(param)
		require.True(t, ok, "%s.%s", stepName, param)
		require.NotEmpty(t, p.Enum)
		return p
	}

	for _, v := range mustFind("process.start", "windowStyle").Enum {
		_, ok := procmgmt.ParseWindowStyle(v)
		assert.True(t, ok, "windowStyle %q", v)
	}
	for _, v := range mustFind("process.start", "priorityClass").Enum {
		_, ok := procmgmt.ParsePriorityClass(v)
		assert.True(t, ok, "priorityClass %q", v)
	}
	for _, v := range mustFind("process.start", "timeoutAction").Enum {
		_, ok := procmgmt.ParseTimeoutAction(v)
		assert.True(t, ok, "timeoutAction %q", v)
	}
	for _, v := range mustFind("process.start", "streamEncoding").Enum {
		assert.True(t, procmgmt.ValidStreamEncoding(v), "streamEncoding %q", v)
	}
	// Session deployMode.
	found := false
	for _, spec := range sessionParamSpecs {
		if spec.Name == "deployMode" {
			found = true
			for _, v := range spec.Enum {
				_, ok := deploy.ParseDeployMode(v)
				assert.True(t, ok, "deployMode %q", v)
			}
		}
	}
	assert.True(t, found)
	for _, v := range mustFind("shortcut.create", "windowStyle").Enum {
		_, err := shortcut.ParseWindowStyle(v)
		assert.NoError(t, err, "shortcut windowStyle %q", v)
	}
	for _, v := range mustFind("service.setStartMode", "startMode").Enum {
		_, err := svcmgmt.ParseStartMode(v)
		assert.NoError(t, err, "startMode %q", v)
	}
	for _, v := range mustFind("registry.set", "type").Enum {
		assert.NotEqual(t, 0, int(registryKindFromString(v)), "registry kind %q must not fall back to inferred", v)
	}
}

// TestBindSmoke calls every step's Bind with minimal valid params and asserts
// a non-nil PhaseFunc without invoking it.
func TestBindSmoke(t *testing.T) {
	// Minimal required params per step (only steps with required params).
	minimal := map[string]map[string]any{
		"msi.install":                 {"path": "x.msi"},
		"msi.uninstall":               {"path": "x.msi"},
		"msi.repair":                  {"path": "x.msi"},
		"msi.patch":                   {"path": "x.msp"},
		"msi.installAsUser":           {"path": "x.msi"},
		"msi.uninstallAsUser":         {"path": "x.msi"},
		"msu.install":                 {"directory": "Updates"},
		"process.start":               {"filePath": "x.exe"},
		"process.startAsUser":         {"filePath": "x.exe"},
		"process.blockExecution":      {"processes": []string{"x"}},
		"process.sendKeys":            {"windowTitle": "t", "keys": "k"},
		"dialog.prompt":               {"message": "m", "buttonRightText": "OK"},
		"dialog.dialogBox":            {"text": "t"},
		"dialog.balloonTip":           {"text": "t"},
		"dialog.notifyIcon":           {"text": "t"},
		"file.copy":                   {"path": []string{"a"}, "destination": "b"},
		"file.remove":                 {"path": []string{"a"}},
		"file.copyToUserProfiles":     {"path": []string{"a"}, "destination": "b"},
		"file.removeFromUserProfiles": {"path": []string{"a"}},
		"folder.create":               {"path": "a"},
		"folder.remove":               {"path": "a"},
		"zip.create":                  {"path": []string{"a"}, "destination": "b.zip"},
		"ini.set":                     {"filePath": "a.ini", "section": "s", "key": "k"},
		"ini.remove":                  {"filePath": "a.ini", "section": "s", "key": "k"},
		"ini.removeSection":           {"filePath": "a.ini", "section": "s"},
		"permission.set":              {"path": "a", "user": "u", "permission": "Read"},
		"registry.set":                {"key": `HKLM:\X`},
		"registry.remove":             {"key": `HKLM:\X`},
		"shortcut.create":             {"path": "a.lnk", "targetPath": "b.exe"},
		"shortcut.removeDesktop":      {"name": "n"},
		"service.start":               {"name": "svc"},
		"service.stop":                {"name": "svc"},
		"service.setStartMode":        {"name": "svc", "startMode": "Manual"},
		"env.set":                     {"variable": "V", "value": "x"},
		"env.remove":                  {"variable": "V"},
		"fonts.add":                   {"filePath": "f.ttf"},
		"fonts.remove":                {"name": "Font"},
		"dll.register":                {"filePath": "a.dll"},
		"dll.unregister":              {"filePath": "a.dll"},
		"regsvr32.invoke":             {"filePath": "a.dll", "action": "register"},
		"activeSetup.set":             {"stubExePath": "stub.exe"},
		"edge.extensionAdd":           {"extensionId": "id", "updateUrl": "u", "installationMode": "allowed"},
		"edge.extensionRemove":        {"extensionId": "id"},
		"sccm.invokeTask":             {"scheduleId": "HardwareInventory"},
		"process.sendkeys":            nil, // placeholder guard, not a real step
		"wim.mount":                   {"imagePath": "x.wim", "path": "mnt"},
		"wim.dismount":                {"path": "mnt"},
	}
	delete(minimal, "process.sendkeys")

	for _, spec := range Steps() {
		params := Params{}
		for name, val := range minimal[spec.Name] {
			params.set(name, Value{V: val})
		}
		fn, err := spec.Bind(params)
		require.NoError(t, err, spec.Name)
		require.NotNil(t, fn, spec.Name)
	}
}
