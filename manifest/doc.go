// Package manifest is the workflow layer over the adt SDK: deployments
// defined as YAML manifests composed from a typed step catalog, validated
// and executed by the adt binary (or programmatically by tooling such as CI
// pipelines and authoring UIs).
//
// # Manifest format (apiVersion v0.1.0-alpha)
//
// A manifest is a deployment.yaml at the package root, beside Files/ and
// the other scaffold directories:
//
//	apiVersion: v0.1.0-alpha
//	kind: Deployment
//	session:
//	  appVendor: Contoso
//	  appName: Widget
//	  appVersion: "1.2.3"
//	phases:
//	  preInstall:
//	    - uses: dialog.welcome
//	      with: {closeProcesses: [{name: widget}], allowDefer: true, deferTimes: 3}
//	  install:
//	    - uses: msi.install
//	      with: {path: Widget.msi, transforms: [Widget.mst]}
//
// The session block is a curated mirror of adt.SessionOptions; phases hold
// the nine PSADT phase slots, each a list of steps. A step names a catalog
// entry (uses), its parameters (with), an optional display name and
// continueOnError. Durations are Go strings ("90s", "1h30m"); timestamps are
// RFC 3339 or YYYY-MM-DD; enums match case-insensitively; unknown or
// duplicate keys are errors everywhere.
//
// # Schema
//
// The format is recorded as a JSON Schema (draft 2020-12) generated from the
// live registry by [JSONSchema] and checked in as [SchemaFileName]
// (winadt.schema-v<semver>.json; a test keeps the two in lockstep —
// regenerate with ADT_UPDATE_SCHEMA=1). [SchemaVersion] tracks the artifact
// with semantic versioning, independent of the manifest apiVersion. `adt
// schema` prints it for editor integration via yaml-language-server. The
// schema captures structure, types, required parameters and enums; the
// semantic, package-file and platform layers exist only in the Go validator.
//
// # Step catalog
//
// [Steps] enumerates the catalog ([Lookup] resolves one): every entry is a
// [StepSpec] with a typed parameter schema, platform tags, semantic checks
// and a binding onto the adt engine. `adt steps` renders the same data on
// the command line.
//
// # Validation
//
// [Load] parses and schema-validates; [Validate] layers semantic checks,
// package-file existence (relative paths against <pkg>/Files/) and platform
// support on top. Issues carry stable codes, logical paths and file
// positions; [NewReport] assembles the machine-readable result `adt
// validate --json` emits. Validation is fully portable — manifests are
// checked on any OS, so CI can gate them without a Windows runner.
//
// # Execution
//
// [Compile] materializes a validated manifest into an *adt.Deployment:
// session params become SessionOptions, each phase's steps are bound and
// chained into a PhaseFunc that logs progress and honors continueOnError,
// and relative package-file parameters are normalized against Files/.
// `adt run` uses [LoadAndCompile] when a package contains a deployment.yaml.
package manifest
