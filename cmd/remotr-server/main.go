package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/gitsync"
	"github.com/DavidHoenisch/remotr/internal/performance"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/server"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

func main() {
	listen := envOr("REMOTR_LISTEN", ":8443")
	repo := envOr("REMOTR_CONFIG_REPO", "/config-repo")
	releaseRef := envOr("REMOTR_RELEASE_REF", "dev")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	diagnosticsAddress, err := performanceDiagnosticsAddress(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if diagnosticsAddress != "" {
		if err := startPerformanceDiagnostics(ctx, diagnosticsAddress); err != nil {
			log.Fatal(err)
		}
	}

	enroller, pgStore := openRegistry()
	admin := openAdmin(enroller)
	deploymentTokens := openDeploymentTokens(enroller, pgStore)
	if pgStore == nil {
		log.Fatal("Remotr Change control requires REMOTR_DATABASE_URL for the durable server registry")
	}
	secretEnvelope, err := loadSecretEnvelopeFromEnvironment(os.Getenv, 0)
	if err != nil {
		log.Fatal(err)
	}
	if secretEnvelope != nil {
		if err := validateRestoredSecretKeyCoverage(ctx, pgStore, secretEnvelope); err != nil {
			log.Fatal(err)
		}
	}
	changes, err := changecontrol.NewPersistentRegistry(ctx, pgStore, changecontrol.RegistryOptions{})
	if err != nil {
		log.Fatal(err)
	}
	changePlans := &server.ChangePlanDeriver{ConfigRepoPath: repo, ArtifactStore: pgStore, StateReports: pgStore}
	var secretRegistry *secrets.RegistryService
	if secretEnvelope != nil {
		if pgStore == nil {
			log.Fatal("Remotr secrets require REMOTR_DATABASE_URL for the encrypted server registry")
		}
		coordinator := server.NewSecretActivationCoordinator(changes, changePlans)
		secretRegistry, err = secrets.NewRegistryService(pgStore, secretEnvelope, coordinator, coordinator)
		if err != nil {
			log.Fatal(err)
		}
		changePlans.Secrets = secretRegistry
	}

	gitSyncer := newGitSyncer(repo, releaseRef, pgStore)
	if pgStore != nil {
		comp := &server.CompositionService{RepoRoot: repo, Store: pgStore}
		gitSyncer.Composer = comp.ComposeAll
	}
	if err := gitSyncer.Sync(ctx); err != nil {
		slog.Error("initial git sync", "err", err)
	}

	caCert, caKey, caPEM, err := tlsconfig.LoadCAKeyPair(
		envOr("REMOTR_CA_CERT", "/certs/ca.crt"),
		envOr("REMOTR_CA_KEY", "/certs/ca.key"),
	)
	if err != nil {
		log.Fatal(err)
	}

	bootstrapFile := envOr("REMOTR_BOOTSTRAP_FILE", "/var/lib/remotr/bootstrap.token")
	bootstrap := server.NewBootstrap(bootstrapFile)
	if err := bootstrap.MaybeInit(admin); err != nil {
		log.Fatal(err)
	}
	fastPathConfig, err := fastPathConfigFromEnvironment(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	srvCfg := server.Config{
		ConfigRepoPath:    repo,
		ReleaseRef:        releaseRef,
		ReleaseRefSrc:     gitSyncer,
		Registry:          enroller,
		Enroller:          enroller,
		Admin:             admin,
		DeploymentTokens:  deploymentTokens,
		Bootstrap:         bootstrap,
		CACert:            caCert,
		CAKey:             caKey,
		CACertPEM:         caPEM,
		GitWebhook:        gitSyncer.Handler(),
		GitSync:           gitSyncer.Sync,
		SyncMaxConcurrent: envInt("REMOTR_SYNC_MAX_CONCURRENT", 0),
		SyncRetryAfter:    envDuration("REMOTR_SYNC_RETRY_AFTER", 5*time.Second),
		FastPath:          fastPathConfig,
		ChangeControl:     changes,
		Secrets:           secretRegistry,
		SecretRegistry:    secretRegistry,
	}
	if pgStore != nil {
		srvCfg.ArtifactStore = pgStore
	} else {
		srvCfg.ArtifactStore = &server.OnDemandArtifactResolver{RepoRoot: repo}
	}
	if pgStore != nil {
		srvCfg.FleetSettings = pgStore
		srvCfg.FleetSettingsMutator = pgStore
		srvCfg.Telemetry = pgStore
		srvCfg.CronScheduler = pgStore
		srvCfg.StateReports = pgStore
		srvCfg.AuditLog = pgStore
		srvCfg.RBAC = pgStore
		srvCfg.AppPackages = pgStore
		srvCfg.Diagnostics = pgStore
		if err := pgStore.EnsureBuiltInRoles(ctx); err != nil {
			log.Fatal(err)
		}
	} else if mem, ok := enroller.(*registry.Memory); ok {
		srvCfg.FleetSettings = mem
		srvCfg.StateReports = mem
	}

	if s3Cfg, ok := apppackages.S3ConfigFromEnv(); ok {
		blobs, err := apppackages.NewBlobStore(ctx, s3Cfg)
		if err != nil {
			log.Fatal(err)
		}
		srvCfg.AppPackageBlobs = blobs
		ttl := envDuration("REMOTR_S3_PRESIGN_TTL", 30*time.Minute)
		srvCfg.AppPackagePresignTTL = ttl
		if srvCfg.AppPackages != nil {
			srvCfg.AppPackageURLs = &apppackages.Service{
				Catalog:    srvCfg.AppPackages,
				Blobs:      blobs,
				PresignTTL: ttl,
			}
		}
	}

	srv := server.New(srvCfg)
	gitSyncer.BeginMutation = srv.BeginGlobalFastPathMutation
	gitSyncer.StartPoll(ctx)
	fastPathStatus := srv.FastPathStatus()
	slog.Info("unchanged Sync fast path", "enabled", fastPathStatus.Enabled, "backend", fastPathStatus.Backend, "reason", fastPathStatus.Reason)

	tlsCfg, err := tlsconfig.ServerTLSConfig(
		envOr("REMOTR_TLS_CERT", "/certs/server.crt"),
		envOr("REMOTR_TLS_KEY", "/certs/server.key"),
		envOr("REMOTR_TLS_CLIENT_CA", "/certs/ca.crt"),
	)
	if err != nil {
		log.Fatal(err)
	}

	https := &http.Server{
		Addr:              listen,
		Handler:           srv.Handler(),
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("remotr-server listening", "addr", listen)
	if err := https.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}

type restoredSecretRecordSource interface {
	ListEncryptedSecretRecords(context.Context) ([]secrets.EncryptedRecord, error)
}

func validateRestoredSecretKeyCoverage(ctx context.Context, source restoredSecretRecordSource, envelope *secrets.Envelope) error {
	if source == nil || envelope == nil {
		return errors.New("restored secret key coverage requires database and keyring inputs")
	}
	records, err := source.ListEncryptedSecretRecords(ctx)
	if err != nil {
		return errors.New("load restored encrypted secret records for key coverage")
	}
	report, err := envelope.CheckKeyCoverage(ctx, records)
	if err != nil {
		return fmt.Errorf("validate restored encrypted secret records: %w", err)
	}
	if !report.Complete {
		return fmt.Errorf("restored database cannot recover %d encrypted secret version(s); install every referenced external KEK", len(report.Missing))
	}
	return nil
}

func loadSecretEnvelopeFromEnvironment(getenv func(string) string, requiredUID uint32) (*secrets.Envelope, error) {
	enabled, err := strconv.ParseBool(strings.TrimSpace(getenv("REMOTR_SECRETS_ENABLED")))
	if err != nil && strings.TrimSpace(getenv("REMOTR_SECRETS_ENABLED")) != "" {
		return nil, fmt.Errorf("REMOTR_SECRETS_ENABLED must be a boolean")
	}
	if !enabled {
		return nil, nil
	}

	path := strings.TrimSpace(getenv("REMOTR_SECRET_KEK_KEYRING"))
	encoded := strings.TrimSpace(getenv("REMOTR_SECRET_KEK_KEYRING_B64"))
	if path == "" && encoded == "" {
		return nil, fmt.Errorf("Remotr secrets are enabled but no external KEK keyring is configured")
	}
	if path != "" && encoded != "" {
		return nil, fmt.Errorf("configure exactly one external KEK keyring source")
	}
	var keyring *secrets.Keyring
	if path != "" {
		keyring, err = secrets.LoadKeyringFile(path, secrets.WithKeyringRequiredUID(requiredUID))
	} else {
		data, decodeErr := base64.StdEncoding.Strict().DecodeString(encoded)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode external KEK keyring environment input: invalid base64")
		}
		keyring, err = secrets.LoadKeyringJSON(data)
		clear(data)
	}
	if err != nil {
		return nil, fmt.Errorf("load external KEK keyring: %w", err)
	}
	return secrets.NewEnvelope(keyring)
}

func openRegistry() (registry.Enroller, *pgstore.Store) {
	if dbURL := os.Getenv("REMOTR_DATABASE_URL"); dbURL != "" {
		st, err := pgstore.New(context.Background(), dbURL)
		if err != nil {
			log.Fatal(err)
		}
		return &pgstore.RegistryEnroller{Store: st}, st
	}
	return registry.NewMemory(), nil
}

func openDeploymentTokens(enroller registry.Enroller, store *pgstore.Store) registry.DeploymentTokens {
	if store != nil {
		return &pgstore.RegistryDeploymentTokens{Store: store}
	}
	if mem, ok := enroller.(*registry.Memory); ok {
		return mem
	}
	return nil
}

func newGitSyncer(repoPath, staticRef string, store *pgstore.Store) *gitsync.GitSyncer {
	poll := envDuration("REMOTR_GIT_SYNC_POLL_INTERVAL", 0)
	gs := &gitsync.GitSyncer{
		RepoPath:      repoPath,
		RemoteURL:     os.Getenv("REMOTR_GIT_REMOTE_URL"),
		Branch:        envOr("REMOTR_GIT_BRANCH", "main"),
		Token:         os.Getenv("REMOTR_GIT_TOKEN"),
		Username:      os.Getenv("REMOTR_GIT_USERNAME"),
		PollInterval:  poll,
		WebhookSecret: os.Getenv("REMOTR_GIT_WEBHOOK_SECRET"),
		StaticRef:     staticRef,
	}
	if store != nil {
		gs.Store = store
	}
	return gs
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("invalid duration env", "key", key, "value", v, "err", err)
		return fallback
	}
	return d
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		slog.Warn("invalid integer env", "key", key, "value", v, "err", err)
		return fallback
	}
	return n
}

func openAdmin(enroller registry.Enroller) registry.Admin {
	switch r := enroller.(type) {
	case *pgstore.RegistryEnroller:
		return &pgstore.RegistryAdmin{Store: r.Store}
	case *registry.Memory:
		return r
	default:
		return nil
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fastPathConfigFromEnvironment(getenv func(string) string) (server.FastPathConfig, error) {
	config := server.FastPathConfig{Enabled: true, Backend: server.FastPathMemory, ServingProcesses: 1, CheckpointInterval: 5 * time.Minute}
	if raw := strings.ToLower(strings.TrimSpace(getenv("REMOTR_UNCHANGED_SYNC_BACKEND"))); raw != "" {
		switch server.FastPathBackend(raw) {
		case server.FastPathDisabled, server.FastPathMemory, server.FastPathRedis:
			config.Backend = server.FastPathBackend(raw)
			config.Enabled = config.Backend != server.FastPathDisabled
		default:
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_BACKEND must be disabled, memory, or redis")
		}
	}
	if raw := strings.TrimSpace(getenv("REMOTR_UNCHANGED_SYNC_FAST_PATH")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_FAST_PATH must be a boolean")
		}
		config.Enabled = enabled
		if !enabled {
			config.Backend = server.FastPathDisabled
		}
	}
	if config.Backend == server.FastPathRedis {
		config.RedisURL = strings.TrimSpace(getenv("REMOTR_REDIS_URL"))
		config.RedisPrefix = strings.TrimSpace(getenv("REMOTR_UNCHANGED_SYNC_REDIS_PREFIX"))
		if config.RedisURL == "" {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_REDIS_URL is required for redis backend")
		}
		if config.RedisPrefix == "" {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_REDIS_PREFIX is required for redis backend")
		}
		parsed, err := url.Parse(config.RedisURL)
		if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" || parsed.User == nil {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_REDIS_URL must be an authenticated redis or rediss URL")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`).MatchString(config.RedisPrefix) {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_REDIS_PREFIX is invalid")
		}
	}
	if raw := strings.TrimSpace(getenv("REMOTR_SERVER_PROCESSES")); raw != "" {
		servingProcesses, err := strconv.Atoi(raw)
		if err != nil || servingProcesses < 1 {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_SERVER_PROCESSES must be a positive integer")
		}
		config.ServingProcesses = servingProcesses
	}
	if config.Backend == server.FastPathMemory && config.ServingProcesses > 1 {
		return server.FastPathConfig{}, fmt.Errorf("memory backend requires one serving process")
	}
	if raw := strings.TrimSpace(getenv("REMOTR_UNCHANGED_SYNC_CHECKPOINT_INTERVAL")); raw != "" {
		interval, err := time.ParseDuration(raw)
		if err != nil || interval < 5*time.Minute || interval > 10*time.Minute {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_CHECKPOINT_INTERVAL must be between 5m and 10m")
		}
		config.CheckpointInterval = interval
	}
	for key, target := range map[string]*int{
		"REMOTR_UNCHANGED_SYNC_MAX_ENTRIES": &config.MaxEntries,
		"REMOTR_UNCHANGED_SYNC_MAX_BYTES":   &config.MaxBytes,
	} {
		if raw := strings.TrimSpace(getenv(key)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 {
				return server.FastPathConfig{}, fmt.Errorf("%s must be a positive integer", key)
			}
			*target = value
		}
	}
	if raw := strings.TrimSpace(getenv("REMOTR_UNCHANGED_SYNC_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return server.FastPathConfig{}, fmt.Errorf("REMOTR_UNCHANGED_SYNC_TTL must be a positive duration")
		}
		config.TTL = ttl
	}
	return config, nil
}

func performanceDiagnosticsAddress(getenv func(string) string) (string, error) {
	address := strings.TrimSpace(getenv("REMOTR_PERFORMANCE_DIAGNOSTICS_ADDR"))
	if address == "" {
		return "", nil
	}
	if err := performance.ValidateDiagnosticsAddress(address); err != nil {
		return "", err
	}
	return address, nil
}

func startPerformanceDiagnostics(ctx context.Context, address string) error {
	if err := performance.StartDiagnostics(ctx, address); err != nil {
		return err
	}
	slog.Info("performance diagnostics listening", "addr", address)
	return nil
}
