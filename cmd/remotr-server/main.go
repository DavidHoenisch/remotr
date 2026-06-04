package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/DavidHoenisch/remotr/internal/gitsync"
	"github.com/DavidHoenisch/remotr/internal/registry"
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

	gitSyncer := newGitSyncer(repo, releaseRef, pgStore)
	gitSyncer.StartPoll(ctx)
	if err := gitSyncer.Sync(ctx); err != nil {
		slog.Warn("initial git sync", "err", err)
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
		ConfigRepoPath: repo,
		ReleaseRef:     releaseRef,
		ReleaseRefSrc:  gitSyncer,
		Registry:       enroller,
		Enroller:       enroller,
		Admin:            admin,
		DeploymentTokens: deploymentTokens,
		Bootstrap:        bootstrap,
		CACert:         caCert,
		CAKey:          caKey,
		CACertPEM:      caPEM,
		GitWebhook:     gitSyncer.Handler(),
		GitSync:        gitSyncer.Sync,
	}
	if pgStore != nil {
		srvCfg.FleetSettings = pgStore
		srvCfg.Telemetry = pgStore
		srvCfg.StateReports = pgStore
	} else if mem, ok := enroller.(*registry.Memory); ok {
		srvCfg.FleetSettings = mem
		srvCfg.StateReports = mem
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
