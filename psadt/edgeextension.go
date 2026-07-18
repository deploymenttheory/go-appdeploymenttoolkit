package psadt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// edgePolicyKey is the Microsoft Edge policy key that stores the
// ExtensionSettings blob both edge-extension functions manage.
const edgePolicyKey = `HKLM\SOFTWARE\Policies\Microsoft\Edge`

// edgeExtensionSettingsValue is the registry value name holding the
// JSON-encoded per-extension settings map.
const edgeExtensionSettingsValue = "ExtensionSettings"

// edgeInstallationModes is the set of installation modes Add-ADTEdgeExtension
// accepts (ValidateSet in the PowerShell source).
var edgeInstallationModes = map[string]struct{}{
	"blocked":          {},
	"allowed":          {},
	"removed":          {},
	"force_installed":  {},
	"normal_installed": {},
}

// edgeExtension is one entry of the ExtensionSettings map. Field order and the
// json tags reproduce the object Add-ADTEdgeExtension composes.
type edgeExtension struct {
	InstallationMode       string `json:"installation_mode"`
	UpdateURL              string `json:"update_url"`
	MinimumVersionRequired string `json:"minimum_version_required,omitempty"`
}

// AddADTEdgeExtensionOptions mirrors the parameters of Add-ADTEdgeExtension.
type AddADTEdgeExtensionOptions struct {
	// ExtensionID is the ID of the extension to add.
	ExtensionID string
	// UpdateURL is the update URL where the extension checks for updates.
	UpdateURL string
	// InstallationMode is one of blocked, allowed, removed, force_installed
	// or normal_installed.
	InstallationMode string
	// MinimumVersionRequired optionally pins a minimum extension version.
	MinimumVersionRequired string
}

// RemoveADTEdgeExtensionOptions mirrors the parameters of
// Remove-ADTEdgeExtension.
type RemoveADTEdgeExtensionOptions struct {
	// ExtensionID is the ID of the extension to remove.
	ExtensionID string
}

// AddADTEdgeExtension is the Go port of Add-ADTEdgeExtension: it force-installs
// (or otherwise configures) a Microsoft Edge extension by merging it into the
// ExtensionSettings policy value under
// HKLM\SOFTWARE\Policies\Microsoft\Edge.
func AddADTEdgeExtension(ctx context.Context, opts AddADTEdgeExtensionOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: AddADTEdgeExtension: %w", err)
	}
	if strings.TrimSpace(opts.ExtensionID) == "" {
		return fmt.Errorf("psadt: ExtensionID is required: %w", ErrInvalidOption)
	}
	if _, ok := edgeInstallationModes[opts.InstallationMode]; !ok {
		return fmt.Errorf("psadt: InstallationMode %q is not valid: %w", opts.InstallationMode, ErrInvalidOption)
	}
	if !isWellFormedAbsoluteURL(opts.UpdateURL) {
		return fmt.Errorf("psadt: UpdateUrl %q is not a valid URL: %w", opts.UpdateURL, ErrInvalidOption)
	}
	msg := fmt.Sprintf("Adding extension with ID [%s] using installation mode [%s] and update URL [%s]",
		opts.ExtensionID, opts.InstallationMode, opts.UpdateURL)
	if opts.MinimumVersionRequired != "" {
		msg += fmt.Sprintf(" with minimum version required [%s]", opts.MinimumVersionRequired)
	}
	logToSession(msg+".", LogSeverityInfo, "AddADTEdgeExtension")

	current, err := getEdgeExtensions(ctx)
	if err != nil {
		return err
	}
	current[opts.ExtensionID] = edgeExtension{
		InstallationMode:       opts.InstallationMode,
		UpdateURL:              opts.UpdateURL,
		MinimumVersionRequired: opts.MinimumVersionRequired,
	}
	return writeEdgeExtensions(ctx, current)
}

// RemoveADTEdgeExtension is the Go port of Remove-ADTEdgeExtension: it removes
// an extension from the ExtensionSettings policy value, doing nothing when the
// extension is not configured.
func RemoveADTEdgeExtension(ctx context.Context, opts RemoveADTEdgeExtensionOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: RemoveADTEdgeExtension: %w", err)
	}
	if strings.TrimSpace(opts.ExtensionID) == "" {
		return fmt.Errorf("psadt: ExtensionID is required: %w", ErrInvalidOption)
	}
	logToSession(fmt.Sprintf("Removing extension with ID [%s].", opts.ExtensionID),
		LogSeverityInfo, "RemoveADTEdgeExtension")

	current, err := getEdgeExtensions(ctx)
	if err != nil {
		return err
	}
	if _, ok := current[opts.ExtensionID]; !ok {
		logToSession(fmt.Sprintf("Extension with ID [%s] is not configured. Removal not required.", opts.ExtensionID),
			LogSeverityInfo, "RemoveADTEdgeExtension")
		return nil
	}
	delete(current, opts.ExtensionID)
	return writeEdgeExtensions(ctx, current)
}

// getEdgeExtensions ports the private Get-ADTEdgeExtensions helper: it reads
// and decodes the current ExtensionSettings value, returning an empty map when
// the value is absent or empty.
func getEdgeExtensions(ctx context.Context) (map[string]edgeExtension, error) {
	present, err := TestADTRegistryValue(ctx, TestADTRegistryValueOptions{
		Key:  edgePolicyKey,
		Name: edgeExtensionSettingsValue,
	})
	if err != nil {
		return nil, err
	}
	if !present {
		return map[string]edgeExtension{}, nil
	}
	raw, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{
		Key:  edgePolicyKey,
		Name: edgeExtensionSettingsValue,
	})
	if err != nil {
		if errors.Is(err, winerr.ErrNotFound) {
			return map[string]edgeExtension{}, nil
		}
		return nil, err
	}
	s, ok := raw.(string)
	if !ok {
		return map[string]edgeExtension{}, nil
	}
	return parseEdgeExtensions(s)
}

// writeEdgeExtensions encodes the extensions map and writes it back to the
// policy value.
func writeEdgeExtensions(ctx context.Context, exts map[string]edgeExtension) error {
	encoded, err := marshalEdgeExtensions(exts)
	if err != nil {
		return err
	}
	return SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
		Key:   edgePolicyKey,
		Name:  edgeExtensionSettingsValue,
		Value: encoded,
	})
}

// parseEdgeExtensions decodes the ExtensionSettings JSON blob, treating empty
// or "{}" content as no configured extensions.
func parseEdgeExtensions(data string) (map[string]edgeExtension, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "{}" {
		return map[string]edgeExtension{}, nil
	}
	out := map[string]edgeExtension{}
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("psadt: decoding Edge ExtensionSettings: %w", err)
	}
	return out, nil
}

// marshalEdgeExtensions encodes the extensions map as compact JSON with
// deterministic (alphabetical) key ordering, matching PowerShell's
// ConvertTo-Json -Compress output shape.
func marshalEdgeExtensions(exts map[string]edgeExtension) (string, error) {
	ids := make([]string, 0, len(exts))
	for id := range exts {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteByte('{')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(id)
		if err != nil {
			return "", fmt.Errorf("psadt: encoding Edge extension id: %w", err)
		}
		value, err := json.Marshal(exts[id])
		if err != nil {
			return "", fmt.Errorf("psadt: encoding Edge extension: %w", err)
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.String(), nil
}

// isWellFormedAbsoluteURL reports whether s is an absolute URL, mirroring the
// Uri.IsWellFormedUriString(Absolute) validation of Add-ADTEdgeExtension.
func isWellFormedAbsoluteURL(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.IsAbs() && u.Host != ""
}
