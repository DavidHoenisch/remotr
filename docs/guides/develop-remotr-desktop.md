# Develop Remotr Desktop on Linux

Use this guide to run, test, build, and package the native desktop application
from a Remotr checkout. Remotr Desktop development is supported on Linux only.
The desktop is an isolated Wails module under `desktop/`; routine root Go builds
do not compile it.

## Prerequisites

Install these tools before running a desktop target:

- Go 1.26 or newer, matching the root and nested module requirements.
- Node.js 24 and Corepack. The repository invokes pnpm 11.7.0 explicitly; do
  not install or substitute a floating pnpm version.
- A C compiler, `pkg-config`, GTK 3 development files, and WebKitGTK 4.1 or 4.0
  development files.
- `xvfb`, `xauth`, and `x11-utils` for the headless native launch smoke.
- Docker for the isolated Linux/amd64 DEB install and removal smoke.
- Flatpak and `flatpak-builder` for the Linux/amd64 Flatpak bundle and isolated
  install/launch/remove smoke.
- Python 3 for the release-manifest generator and checker.

On Debian or Ubuntu, the native libraries and headless display tools are:

```bash
sudo apt-get update
sudo apt-get install --yes \
  appstream build-essential desktop-file-utils flatpak flatpak-builder pkg-config \
  libgtk-3-dev libwebkit2gtk-4.1-dev \
  xvfb xauth x11-utils

flatpak remote-add --user --if-not-exists \
  flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak install --user --noninteractive \
  flathub org.gnome.Platform//50 org.gnome.Sdk//50
```

On Fedora, install `gtk3-devel` and `webkit2gtk4.1-devel`. On Arch, install
`gtk3` and `webkit2gtk-4.1`. Configure Docker according to your distribution so
your user can run the repository's isolated package smoke.

The prerequisite selector prefers WebKitGTK 4.1 and supplies Wails' required
`webkit2_41` build tag. It falls back to WebKitGTK 4.0 without that tag. A
missing GTK or WebKit package produces a targeted error before Wails starts.

## Install locked dependencies and run tests

From the repository root:

```bash
make desktop-test
```

This installs the frozen frontend lockfile with pnpm 11.7.0, downloads the
pinned nested Go modules, runs the desktop Go suite, and runs frontend unit and
component tests. It does not change the root module's dependency graph.

For the complete frontend gate used by CI, run the commands in
`desktop/frontend/`:

```bash
corepack pnpm@11.7.0 typecheck
corepack pnpm@11.7.0 lint
corepack pnpm@11.7.0 test
corepack pnpm@11.7.0 test:browser
corepack pnpm@11.7.0 build
```

Playwright uses the pinned full Chromium renderer. Its browser installation is
separate from `pnpm install`.

## Run the application

Start Wails development mode from the repository root:

```bash
make desktop-dev
```

The target checks Linux, GTK, and WebKitGTK before starting the Vite watcher and
native window. Use the connection and bootstrap procedures in
[Use Remotr Desktop](use-remotr-desktop.md); do not place test credentials in
frontend fixtures or browser storage.

## Build and launch-smoke a native binary

Build a production-mode executable with an explicit identity:

```bash
make desktop-build DESKTOP_VERSION=0.0.0-local.1
make desktop-smoke DESKTOP_VERSION=0.0.0-local.1
```

The binary is written to `desktop/build/bin/remotr-desktop`. The smoke checks
its exact embedded `Remotr Desktop` version, launches it under Xvfb, observes a
native window titled `Remotr Desktop`, and then terminates the process.

## Build and verify the development DEB

Run the complete advertisement gate:

```bash
make desktop-release-check DESKTOP_VERSION=0.0.0-local.1
```

The target performs these operations in order:

1. Build the production Linux/amd64 Wails binary.
2. Create a root-owned Linux/amd64 DEB with the desktop entry, AppStream
   metadata, and hicolor icon.
3. Install and purge the exact DEB with native `dpkg` in the pinned Debian
   container.
4. Launch the exact packaged executable under Xvfb.
5. Generate and check `desktop/build/package/release-manifest.json`, including
   the file's SHA-256, size, version, target, and lifecycle evidence.

The output is an **unsigned development snapshot**. It is not release eligible
and must not be described as signed output. CI retains the same evidenced DEB
and manifest for seven days as a development artifact.

The manifest check rejects an undeclared file in `desktop/build/package/`. If a
local run reports an undeclared artifact, empty that generated-output directory
and rerun the target with one version. Do not add macOS, Windows, arm64, RPM, or
another format until its exact native lifecycle has its own advertised target
and evidence.

## Build and verify the Flatpak release asset

Run the tagged-release-equivalent Flatpak gate:

```bash
make desktop-flatpak-release-check DESKTOP_VERSION=0.0.0-local.1
```

The target builds the production Wails binary, packages it against
`org.gnome.Platform//50`, installs the exact single-file bundle into an
isolated user installation, verifies its embedded version and exported desktop
metadata, observes its native window under Xvfb, removes it, and checks its
release manifest. Outputs are written beneath
`desktop/build/flatpak-package/`.

The Flatpak grants only network, graphics-device, Wayland/fallback-X11, IPC,
and exact `~/.config/remotr` access, so both the standard configuration and
existing saved absolute profile references keep using the host Operator
configuration and credential directory as their source of truth.
The tagged release publishes the `.flatpak`, `release-manifest.json`, and
`desktop-cli-parity.json`; GoReleaser also includes all three in
`checksums.txt`. The asset is release eligible and explicitly unsigned.

## Run the root regression gate

Before handing off a desktop change, run:

```bash
make test
```

This retains the root fuzz seed corpora and Go suite independently of the
desktop dependency graph. See the [Remotr Desktop support
reference](../reference/remotr-desktop.md) for the current artifact and parity
claims.

## Build troubleshooting

| Symptom | Resolution |
| --- | --- |
| GTK 3 or WebKitGTK is missing | Install the distribution package listed above and confirm `pkg-config --exists gtk+-3.0` plus either `webkit2gtk-4.1` or `webkit2gtk-4.0`. |
| `xvfb-run is required` | Install `xvfb` and `xauth`; install `x11-utils` for `xwininfo`. |
| Docker access is denied | Start Docker and grant the current developer access through the distribution's supported Docker-group or rootless setup. Do not weaken the package smoke. |
| `flatpak-builder is required` | Install Flatpak tooling, add Flathub for the user, and install `org.gnome.Platform//50` plus `org.gnome.Sdk//50`. |
| The Flatpak launch smoke cannot create a sandbox | Enable the distribution's supported unprivileged-user-namespace configuration; do not run the packaged app outside Flatpak to claim lifecycle evidence. |
| The native process exits before opening a window | Read the smoke output, verify the WebKitGTK ABI selected by the prerequisite check, and rerun `make desktop-smoke` with the same version used for the build. |
| The release check reports an undeclared artifact | Remove stale generated files from `desktop/build/package/`; the upload directory must contain only the current DEB and its checked manifest. |
| A frontend visual test changes unexpectedly | Re-run the pinned Chromium suite, inspect all affected states, and update baselines only for an intentional UI change. |
