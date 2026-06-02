package scaffold

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

// Options controls configuration repository scaffolding.
type Options struct {
	Dir                string
	Fleet              string
	RemediationPolicy  string // auto or report
	RegisterServer     bool
	DatabaseURL        string
	CreateEnrollToken  bool
	EnrollTokenTTL     time.Duration
	EnrollTokenOut     string // optional file path
}

// Result summarizes what was created.
type Result struct {
	Dir           string
	Fleet         string
	EnrollToken   string
	EnrollExpires time.Time
}

// Init creates a new Remotr configuration repository layout under opts.Dir.
func Init(ctx context.Context, opts Options) (Result, error) {
	opts.Dir = strings.TrimSpace(opts.Dir)
	if opts.Dir == "" {
		return Result{}, errors.New("output directory is required")
	}
	if opts.Fleet == "" {
		opts.Fleet = "default"
	}
	if err := configrepo.ValidateFleetName(opts.Fleet); err != nil {
		return Result{}, fmt.Errorf("fleet: %w", err)
	}
	policy := strings.TrimSpace(opts.RemediationPolicy)
	if policy == "" {
		policy = "auto"
	}
	switch policy {
	case "auto", "report":
	default:
		return Result{}, fmt.Errorf("remediation policy must be auto or report")
	}

	if err := writeRepoTree(opts.Dir, opts.Fleet, policy); err != nil {
		return Result{}, err
	}

	res := Result{Dir: opts.Dir, Fleet: opts.Fleet}
	if !opts.RegisterServer {
		return res, nil
	}
	dbURL := strings.TrimSpace(opts.DatabaseURL)
	if dbURL == "" {
		return Result{}, errors.New("register server requires database URL (flag or REMOTR_DATABASE_URL)")
	}
	tok, exp, err := registerOnServer(ctx, dbURL, opts)
	if err != nil {
		return Result{}, err
	}
	res.EnrollToken = tok
	res.EnrollExpires = exp
	if opts.EnrollTokenOut != "" && tok != "" {
		if err := os.WriteFile(opts.EnrollTokenOut, []byte(tok+"\n"), 0o600); err != nil {
			return Result{}, fmt.Errorf("write enroll token file: %w", err)
		}
	}
	return res, nil
}

func writeRepoTree(dir, fleet, policy string) error {
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s is not empty; choose an empty directory", dir)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	dirs := []string{
		dir,
		filepath.Join(dir, "fleets", fleet),
		filepath.Join(dir, "endpoints"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil { // #nosec G301 -- public scaffold tree
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	files := map[string]string{
		filepath.Join(dir, ".gitignore"): gitignoreContent,
		filepath.Join(dir, "README.md"):  readmeContent(fleet, policy),
		filepath.Join(dir, "remotr.yaml"): remotrMetaContent(fleet, policy),
		filepath.Join(dir, "server.env.example"): serverEnvExample(dir, fleet),
		filepath.Join(dir, "fleets", fleet, "desired.yaml"): sampleDesiredYAML(),
		filepath.Join(dir, "endpoints", ".gitkeep"):       "",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil { // #nosec G306 -- public template files
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func registerOnServer(ctx context.Context, dbURL string, opts Options) (token string, expires time.Time, err error) {
	st, err := pgstore.New(ctx, dbURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("connect database: %w", err)
	}

	policy := strings.TrimSpace(opts.RemediationPolicy)
	if policy == "" {
		policy = "auto"
	}
	if err := st.SetRemediationPolicy(ctx, opts.Fleet, policy); err != nil {
		return "", time.Time{}, fmt.Errorf("fleet settings: %w", err)
	}

	if !opts.CreateEnrollToken {
		return "", time.Time{}, nil
	}
	ttl := opts.EnrollTokenTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token = hex.EncodeToString(raw)
	expires = time.Now().UTC().Add(ttl)
	if _, err := st.CreateEnrollmentToken(ctx, token, opts.Fleet, expires); err != nil {
		return "", time.Time{}, fmt.Errorf("create enrollment token: %w", err)
	}
	return token, expires, nil
}

func sampleDesiredYAML() string {
	const sample = `configurations:
  - name: base-packages
    description: Baseline packages for this fleet (edit for your org)
    targetDistros:
      - Debian
      - Arch
    packages:
      - name: curl
        present: true
        packageManager: apt
      - name: curl
        present: true
        packageManager: pacman
`
	// Validate before shipping template.
	var stub struct {
		Configurations []any `yaml:"configurations"`
	}
	if err := yaml.Unmarshal([]byte(sample), &stub); err != nil {
		panic("invalid sample desired.yaml: " + err.Error())
	}
	return sample
}

const gitignoreContent = `# Local operator notes (optional)
*.local.env
.server.env

# Editor
.idea/
.vscode/
`

func remotrMetaContent(fleet, policy string) string {
	meta := map[string]any{
		"version": 1,
		"kind":    "remotr-config-repo",
		"defaultFleet": fleet,
		"fleets": []map[string]any{
			{
				"name":               fleet,
				"remediationPolicy": policy,
				"artifact":           fmt.Sprintf("fleets/%s/desired.yaml", fleet),
			},
		},
		"paths": map[string]string{
			"fleetArtifact":     "fleets/<fleet>/desired.yaml",
			"endpointOverride": "endpoints/<endpoint-id>/desired.yaml",
		},
	}
	b, err := yaml.Marshal(meta)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func readmeContent(fleet, policy string) string {
	return fmt.Sprintf(`# Remotr configuration repository

GitOps source of truth for desired state. The Remotr server serves files from this
repository at a **release ref**; agents never pull Git directly.

## Layout

- `+"`fleets/%s/desired.yaml`"+` — deployable artifact for fleet **%s**
- `+"`endpoints/<endpoint-id>/desired.yaml`"+` — optional per-machine override (replaces fleet file)
- `+"`remotr.yaml`"+` — operator metadata (not served to agents)
- `+"`server.env.example`"+` — suggested server environment variables

## Fleet **%s**

Remediation policy on the server registry: **%s** (`+"`auto`"+` applies drift on sync; `+"`report`"+` records only).

## Next steps

1. Initialize Git and push this repository to your forge.
2. Clone or mount it on the Remotr server host and set `+"`REMOTR_CONFIG_REPO`"+` to the checkout path.
3. Register fleets and enrollment tokens on the server (or re-run `+"`remotr init --register-server`"+`).
4. Enroll agents with a one-time token, then verify sync over mTLS.

## Add another fleet

`+"```bash`"+`
mkdir -p fleets/new-fleet
cp fleets/%s/desired.yaml fleets/new-fleet/desired.yaml
# Register fleet on server (Postgres), then create an enrollment token via your operator workflow.
`+"```"+`

See [Remotr CONTEXT](https://github.com/DavidHoenisch/remotr/blob/master/CONTEXT.md) for domain terms.
`, fleet, fleet, fleet, policy, fleet)
}

func serverEnvExample(repoDir, fleet string) string {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		abs = repoDir
	}
	return fmt.Sprintf(`# Copy to the server host and source before starting remotr-server.
REMOTR_LISTEN=:8443
REMOTR_CONFIG_REPO=%s
REMOTR_RELEASE_REF=main
REMOTR_DATABASE_URL=postgres://remotr:CHANGE_ME@127.0.0.1:5432/remotr?sslmode=disable
REMOTR_TLS_CERT=/etc/remotr/certs/server.crt
REMOTR_TLS_KEY=/etc/remotr/certs/server.key
REMOTR_TLS_CLIENT_CA=/etc/remotr/certs/ca.crt
REMOTR_CA_CERT=/etc/remotr/certs/ca.crt
REMOTR_CA_KEY=/etc/remotr/certs/ca.key

# Fleet "%s" must exist in Postgres fleet_settings (remotr init --register-server).
`, abs, fleet)
}
