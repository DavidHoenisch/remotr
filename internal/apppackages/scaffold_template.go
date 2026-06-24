package apppackages

import (
	"fmt"
	"strings"
)

type manifestTemplateData struct {
	Name          string
	Version       string
	BinName       string
	Mode          string
	VersionMarker string
}

func renderManifestTemplate(name, version, binName, mode string) string {
	data := manifestTemplateData{
		Name:          name,
		Version:       version,
		BinName:       binName,
		Mode:          mode,
		VersionMarker: DefaultVersionFile(name),
	}

	var b strings.Builder
	b.WriteString(`# Custom app package manifest (schemaVersion 1).
# See docs/guides/custom-app-packages.md for the full reference.
#
# This scaffold includes every layout option as examples. Delete or comment out
# sections you do not need. Exactly one install.mode applies at a time.

schemaVersion: 1
name: `)
	b.WriteString(data.Name)
	b.WriteString("\nversion: \"")
	b.WriteString(data.Version)
	b.WriteString("\"\n\ninstall:\n")
	b.WriteString(renderInstallSection(data))
	b.WriteString(`
# Optional: verify the installed app reports the expected version.
check:
  command: ["/usr/local/bin/`)
	b.WriteString(data.BinName)
	b.WriteString(`", "--version"]
  expect: "`)
	b.WriteString(data.Version)
	b.WriteString(`"
  # versionFile: `)
	b.WriteString(data.VersionMarker)
	b.WriteString(`  # default when omitted

# Optional: explicit uninstall steps. Binary mode auto-removes install.files dest
# paths when uninstall is omitted.
# uninstall:
#   files:
#     - /usr/local/bin/`)
	b.WriteString(data.BinName)
	b.WriteString(`
#     - /etc/`)
	b.WriteString(data.BinName)
	b.WriteString(`/`)
	b.WriteString(data.BinName)
	b.WriteString(`.conf
#   script:
#     - ./uninstall.sh
`)
	return b.String()
}

func renderInstallSection(data manifestTemplateData) string {
	switch data.Mode {
	case "script":
		return renderScriptInstall(data)
	case "build":
		return renderBuildInstall(data)
	default:
		return renderBinaryInstall(data)
	}
}

func renderBinaryInstall(data manifestTemplateData) string {
	return fmt.Sprintf(`  mode: binary  # binary | script | build

  # Binary: copy pre-built files from the zip to absolute paths on the endpoint.
  files:
    - src: bin/%[1]s-linux-amd64
      dest: /usr/local/bin/%[2]s
      mode: "0755"
      arch: x86
    - src: bin/%[1]s-linux-arm64
      dest: /usr/local/bin/%[2]s
      mode: "0755"
      arch: ARM
    # - src: share/%[2]s.conf.example
    #   dest: /etc/%[2]s/%[2]s.conf
    #   mode: "0644"
    # - src: lib/%[2]s-helper
    #   dest: /usr/lib/%[2]s/helper
    #   mode: "0755"

  # Script: run commands after extract (set mode: script and remove files above).
  # script:
  #   - ./install.sh

  # Build: run build steps, then script (set mode: build and remove files above).
  # build:
  #   - [python3, -m, venv, .venv]
  #   - [.venv/bin/pip, install, -r, requirements.txt]
  # script:
  #   - ./install.sh
`, data.BinName, data.BinName)
}

func renderScriptInstall(data manifestTemplateData) string {
	return fmt.Sprintf(`  mode: script  # binary | script | build

  # Script: run commands after extract.
  script:
    - ./install.sh

  # Binary: copy pre-built files from the zip (set mode: binary and remove script).
  # files:
  #   - src: bin/%[1]s-linux-amd64
  #     dest: /usr/local/bin/%[2]s
  #     mode: "0755"
  #     arch: x86
  #   - src: bin/%[1]s-linux-arm64
  #     dest: /usr/local/bin/%[2]s
  #     mode: "0755"
  #     arch: ARM
  #   - src: share/%[2]s.conf.example
  #     dest: /etc/%[2]s/%[2]s.conf
  #     mode: "0644"

  # Build: run build steps, then script (set mode: build).
  # build:
  #   - [python3, -m, venv, .venv]
  #   - [.venv/bin/pip, install, -r, requirements.txt]
`, data.BinName, data.BinName)
}

func renderBuildInstall(data manifestTemplateData) string {
	return fmt.Sprintf(`  mode: build  # binary | script | build

  # Build: run build steps in the extracted package directory, then script.
  build:
    - [python3, -m, venv, .venv]
    - [.venv/bin/pip, install, -r, requirements.txt]
  script:
    - ./install.sh

  # Binary: copy pre-built files from the zip (set mode: binary).
  # files:
  #   - src: bin/%[1]s-linux-amd64
  #     dest: /usr/local/bin/%[2]s
  #     mode: "0755"
  #     arch: x86
  #   - src: bin/%[1]s-linux-arm64
  #     dest: /usr/local/bin/%[2]s
  #     mode: "0755"
  #     arch: ARM

  # Script-only install without build steps (set mode: script).
  # script:
  #   - ./install.sh
`, data.BinName, data.BinName)
}
