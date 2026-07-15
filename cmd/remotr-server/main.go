package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/gitsync"
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

	enroller, pgStore := openRegistry()
	admin := openAdmin(enroller)
	deploymentTokens := openDeploymentTokens(enroller, pgStore)
	secretEnvelope, err := loadSecretEnvelopeFromEnvironment(os.Getenv, 0)
	if err != nil {
		log.Fatal(err)
	}
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{})
	var secretRegistry *secrets.RegistryService
	if secretEnvelope != nil {
		if pgStore == nil {
			log.Fatal("Remotr secrets require REMOTR_DATABASE_URL for the encrypted server registry")
		}
		coordinator := server.NewSecretActivationCoordinator(changes)
		secretRegistry, err = secrets.NewRegistryService(pgStore, secretEnvelope, coordinator, coordinator)
		if err != nil {
			log.Fatal(err)
		}
	}

	gitSyncer := newGitSyncer(repo, releaseRef, pgStore)
	if pgStore != nil {
		comp := &server.CompositionService{RepoRoot: repo, Store: pgStore}
		gitSyncer.Composer = comp.ComposeAll
	}
	gitSyncer.StartPoll(ctx)
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
