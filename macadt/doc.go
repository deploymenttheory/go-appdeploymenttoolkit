// Package macadt is the macOS SDK of go-appdeploymenttoolkit, the sibling
// of winadt over the shared deploy engine.
//
// Status: proof-of-life. The session lifecycle (deploy.Open/Close, logging,
// phases, exit-code semantics) already runs on darwin through the engine's
// platform seams; the macOS domain function catalog (pkg/DMG installation,
// defaults/plist, launchd, dialogs) and the manifest step namespaces
// (pkg.*, plist.*, launchd.*) are future increments.
//
// A deployment authored today against this package:
//
//	(&deploy.Deployment{
//	    Session: deploy.SessionOptions{AppVendor: "Contoso", AppName: "Widget", AppVersion: "1.0"},
//	    Install: func(ctx context.Context, s *deploy.Session) error {
//	        return deploy.WriteLogEntry(ctx, deploy.LogEntryOptions{Message: []string{"installing"}})
//	    },
//	}).Run(ctx)
package macadt
