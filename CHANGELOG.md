# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.0 (2026-08-25)


### ⚠ BREAKING CHANGES

* the import path github.com/deploymenttheory/go-appdeploymenttoolkit/adt is now .../winadt (API unchanged via identity aliases; the shared engine is importable as .../deploy). Internal packages moved under internal/shared/ and internal/win/.

### Features

* core deployment session engine (Phases 0-1 of PSADT Go port) ([c2bb6c8](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/c2bb6c840a0fa3ba6956f4f66e03d65c9017ac71))
* cut over to the platform SDKs and retire the adt package ([435e031](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/435e031f8e82923dcc63eabe0727e2b0359dc177))
* examples ([1f5f025](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/1f5f0258f33ee2aa55b5aec6258bc4683be77a1e))
* initial commit ([b9bb824](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/b9bb824e4ef10ab7785951d847f18ee3d17e4be9))
* introduce shared deploy engine and platform SDKs alongside adt ([df6687d](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/df6687d807c57906e87a1dcb36bd78841af19c0f))
* long-tail functions — fonts, active setup, WIM, SCCM, updates, probes (Phase 5) ([a84e0fd](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/a84e0fdd7933cb1ab6b3d6c73bd1e58d6b4e8257))
* migrate to published bindings and wire SCCM via WMI ([a317e59](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/a317e59f4513e200759ad97ac994d82eca36dd31))
* repo refactor ([e69ed37](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/e69ed3740c2abb5b5b7d2d7e16fc1e16f38a2ec1))
* scaffold a commented config.yaml and flesh out examples ([3eea269](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/3eea2699102765845943c34ad164cb4b72230ba7))
* system domains — registry, filesystem, INI, process, MSI, services, shortcuts, users (Phase 2) ([ebc289d](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/ebc289de0be52978088d41c57d17b2c8ba72fd62))
* UI dialogs, cross-session client-server, and CLI runner (Phases 3-4) ([91848b6](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/91848b63e831e36ef45d81bd6f6087e481e92ba9))


### Bug Fixes

* - A. Auto deploy-mode now runs PSADT's full decision chain: OOBE/ESP in progress â NonInteractive, session 0 without an interactive station â Silent, no processes-to-close running (or none specified â strict 4.2 semantics you chose) â Silent. Four new opt-out flags (NoOobeDetection, NoProcessDetection, NoSessionDetection, ProcessInteractivityDetection) are available on SessionOptions, the frontend flags, and adt run. Every decision is logged live, verified in the smoke run. ([d91f2c6](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/d91f2c6246d5ce9fae4fec8ed0808ee1f7d53e22))
* - parity gaps ([6faa822](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/6faa822fad4a80fcd2e4a8348aa8443d4226b148))
* remediate four defects found in Windows runtime validation ([423516a](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/423516a01b9ef768cb1dc67a1656693b27194f3e))
* remediate four defects found in Windows runtime validation ([019917b](https://github.com/deploymenttheory/go-appdeploymenttoolkit/commit/019917bc28952e98b43f68120726675ce121dafa))

## [Unreleased]

### Added

- Added xyz [@your_username](https://github.com/your_username)

### Fixed

- Fixed zyx [@your_username](https://github.com/your_username)

## [1.1.0] - 2021-06-23

### Added

- Added x [@your_username](https://github.com/your_username)

### Changed

- Changed y [@your_username](https://github.com/your_username)

## [1.0.0] - 2021-06-20

### Added

- Inititated y [@your_username](https://github.com/your_username)
- Inititated z [@your_username](https://github.com/your_username)
