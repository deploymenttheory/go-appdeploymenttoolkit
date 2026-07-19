module github.com/deploymenttheory/go-appdeploymenttoolkit

go 1.25.0

require (
	github.com/deploymenttheory/go-bindings-win32 v0.2.0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

// The toolkit tracks go-bindings-win32's current (post-v0.2.0) API, which is
// not yet tagged. Until a matching release is published, resolve it from a
// sibling checkout. Remove this replace and bump the require above once a
// compatible tag exists.
replace github.com/deploymenttheory/go-bindings-win32 => ../go-bindings-win32
