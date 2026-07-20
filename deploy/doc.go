// Package deploy is the platform-neutral deployment engine shared by the
// platform SDKs (winadt today, macadt in future) and the manifest workflow
// layer: session lifecycle, phase dispatch, exit-code semantics, hooks,
// logging and deferral state.
//
// A deployment describes its phases against this engine and runs them with
// [Deployment.Run]; the platform SDKs supply the domain function catalogs
// the phases call. The manifest layer compiles YAML manifests down to the
// same [Deployment]/[PhaseFunc] contract.
//
// Naming: this package uses clean Go names ([Open], [Close], [Current],
// [Session]); winadt re-exports them under their PSADT-parity names
// (OpenADTSession, DeploymentSession, ...) as identical aliases.
//
// Configuration caveat: [Config] aliases the shared toolkit configuration,
// whose field set carries PSADT heritage (registry paths, Fluent accent
// colors). macOS deployments consume the same configuration schema by
// design — platform-inapplicable fields are simply inert there.
package deploy
