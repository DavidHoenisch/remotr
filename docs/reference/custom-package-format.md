# Custom package manifest reference

A Remotr custom application package is a zip archive with
`remotr-package.yaml` at the archive root. The catalog identity in desired
state must exactly match the manifest `name` and `version`.

```text
mycli-1.4.0.zip
├── remotr-package.yaml
└── bin/
    ├── mycli-linux-amd64
    └── mycli-linux-arm64
```

Validate before publishing:

```bash
remotr app package validate ./mycli-1.4.0.zip
```

## Complete binary example

```yaml
schemaVersion: 1
name: internal/mycli
version: 1.4.0

install:
  mode: binary
  files:
    - src: bin/mycli-linux-amd64
      dest: /usr/local/bin/mycli
      mode: "0755"
      arch: x86
    - src: bin/mycli-linux-arm64
      dest: /usr/local/bin/mycli
      mode: "0755"
      arch: ARM

check:
  command: [/usr/local/bin/mycli, --version]
  expect: 1.4.0

uninstall:
  files:
    - /usr/local/bin/mycli
```

## Top-level fields

| Field | Required | Contract |
| --- | --- | --- |
| `schemaVersion` | yes | Must be `1`. |
| `name` | yes | 1–128 characters; begins alphanumeric, then letters, digits, `.`, `_`, `/`, or `-`. |
| `version` | yes | 1–64 characters; begins alphanumeric, then letters, digits, `.`, `_`, `+`, or `-`. |
| `install` | yes | One install mode and its fields. |
| `check` | no | Post-install command and expected stdout substring; `versionFile` is also parsed. |
| `uninstall` | no | Declared files/script. See the current-runtime warning below. |

Package names may include catalog namespaces such as `internal/mycli`. The
default local marker is
`/var/lib/remotr/apps/<sanitized-name>/version`, where `/` becomes `-`.

## Install modes

### `binary`

Copies files from the extracted zip to absolute endpoint paths:

```yaml
install:
  mode: binary
  files:
    - src: bin/tool
      dest: /usr/local/bin/tool
      mode: "0755"
      arch: x86
    - src: share/tool.conf
      dest: /etc/tool/tool.conf
      mode: "0644"
```

| File field | Required | Contract |
| --- | --- | --- |
| `src` | yes | Relative zip member path; absolute and `..` paths are rejected. The member must exist. |
| `dest` | yes | Absolute endpoint path. Parent directories are created mode `0750`. |
| `mode` | no | Quoted octal mode; default `0755`. Parsed on the endpoint. |
| `arch` | no | `x86` or `ARM`; omitted copies on both. |

The server validates referenced members. The endpoint selects files using its
normalized architecture and verifies the catalog zip SHA-256 before extract.

### `script`

Runs one argv array after extraction:

```yaml
install:
  mode: script
  script: [./install.sh, --prefix, /opt/internal/tool]
```

`script` must be non-empty. If argv element zero starts with `./`, Remotr
resolves that executable below the extracted package directory. Arguments are
passed without a shell. The referenced script must exist in the archive.

The command runs as the root agent process. Package scripts therefore have an
unbounded ownership surface; audit the script and avoid embedding credentials
or fetching mutable dependencies.

The current runner resolves a leading `./` executable below the extracted
package directory, but it does **not** change the process working directory.
Relative paths used inside the script therefore resolve from the agent's
working directory, not from the package. Make scripts anchor payload paths to
their own location, for example:

```sh
PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install -m 0755 "$PACKAGE_DIR/bin/tool" /usr/local/bin/tool
```

### `build`

Runs one or more argv-array build steps, followed by an install script:

```yaml
install:
  mode: build
  build:
    - [./build.sh]
  script: [./install.sh]
```

`build` must contain at least one non-empty step. If `script` is omitted, the
endpoint uses `./install.sh`, which must exist in the archive. Prefer absolute
executables in build steps. The current runner does not provide a shell or a
manifest-level working-directory setting.

Build commands have the same working-directory limitation as install scripts.
A wrapper such as `build.sh` must locate the extracted directory from `$0`
before using relative sources. The current script/build scaffolds contain
relative-path starter examples; correct those examples before publishing.

Build mode also makes endpoint convergence depend on compilers and
dependencies installed on each endpoint. Prebuilt `binary` packages are more
reproducible for fleet use.

## Post-install check

```yaml
check:
  command: [/usr/local/bin/mycli, --version]
  expect: 1.4.0
```

When `command` is set, `expect` is required. The command must exit zero and its
stdout must contain the `expect` string. It runs after installation and after
the version marker is written.

!!! warning "A failed post-install check does not remove the marker"
    The current applicator writes the version marker before running this
    command. If the command fails, installed files and the marker remain; the
    next steady-state check examines only the marker and can report the
    package compliant without rerunning `check.command`. Treat a failed check
    as a partial installation: repair or remove the marker and verify the
    payload independently before the next sync.

`check.versionFile` is accepted by manifest validation and must be absolute,
but the current package applicator uses the default marker path when checking
steady state. Do not rely on a custom `versionFile` until the runtime honors
it consistently.

## Removal fields and current behavior

The manifest schema accepts:

```yaml
uninstall:
  files:
    - /usr/local/bin/mycli
    - /etc/mycli/mycli.conf
  script: [./uninstall.sh]
```

Every `uninstall.files` entry must be absolute, and a referenced uninstall
script must exist in the zip. When omitted for binary mode, the manifest model
derives the binary destination paths.

!!! danger "Desired-state removal currently removes only the marker"
    The current endpoint applicator does not execute `uninstall.script` or
    remove `uninstall.files` when a Remotr catalog package becomes absent. It
    removes only the local version marker. The installed payload can remain on
    disk and a later sync can report the package absent.

    Until runtime uninstall is implemented, remove package payloads with a
    separately reviewed typed resource or migration procedure and verify the
    paths independently. Do not claim that catalog removal erased the app.

## Archive validation and safety

- `remotr-package.yaml` must be at the archive root.
- Referenced binary sources and install/uninstall script executables must
  exist. Build-step executables are not prevalidated as archive members and
  can still fail on the endpoint.
- Zip extraction rejects paths escaping the extraction directory.
- The endpoint checks the downloaded zip against the SHA-256 registered by the
  server.
- Upload size is bounded to 256 MiB.
- Catalog download URLs are short-lived server-minted URLs; endpoint and
  operator configurations do not contain S3 credentials.

Archive validation does not establish publisher provenance, inspect script
behavior, validate binary signatures, or make build inputs reproducible. Apply
normal software-supply-chain review before publishing.

## Publish and select

```bash
remotr package build --path ./mycli --output ./mycli-1.4.0.zip
remotr app package validate ./mycli-1.4.0.zip
remotr app publish ./mycli-1.4.0.zip
```

Select it with an application source:

```yaml
kind: application
name: internal/mycli
present: true
packageManager: remotr
version: 1.4.0
```

Or directly from a schema-1 module:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: internal-apps
    resources:
      - kind: package
        name: internal/mycli
        lifecycle: present
        packageManager: remotr
        version: 1.4.0
```

The desired-state name and version must equal the package manifest exactly.

## Scaffold modes

```bash
remotr package create --path ./mycli \
  --name internal/mycli \
  --version 1.4.0 \
  --mode binary
```

Valid scaffold modes are `binary`, `script`, and `build`. The generated
manifest contains commented alternatives; delete unused examples, add the
payload, then build and validate the archive.
