.PHONY: test test-fuzz-seeds vendor fuzz fuzz-short gosec benchmark-capability-variants benchmark-foundation-controlled performance-budget-lint performance-benchmark-gate mutation-policy-lint mutation-capability-selection mutation-high-gate mutation-ubuntu-qualification mutation-ubuntu-pro-management mutation-comprehensive ubuntu-2404-applicator-qualification-audit compose-up compose-down test-e2e test-e2e-quick test-e2e-enroll load-once load-steady-400 load-steady-4000 soak-smoke-400 soak-medium-400 soak-long-400 load-startup-reconnect-400 load-release-fanout-400 load-telemetry-heavy-400 load-capability-mixed-400 load-server-recovery-400 load-postgres-recovery-400 load-policy-shaped-recovery-400 load-overload-400 provider-package-fixtures provider-package-fixtures-reproducible provider-matrix-containers provider-matrix-apt-debian-12 provider-matrix-apt-ubuntu-24-04 provider-matrix-apt-repository-debian-12 provider-matrix-apt-repository-ubuntu-24-04 provider-matrix-pacman-arch-2026-07-06 provider-matrix-pacman-repository-arch-2026-07-06 provider-matrix-aur-arch-2026-07-06 provider-matrix-systemd-timer provider-matrix-systemd-unit provider-matrix-vm-up provider-matrix-vm-restore provider-matrix-vm-destroy provider-matrix-vm-lifecycle provider-matrix-vm-network-recovery provider-matrix-vm-system-safety provider-matrix-vm-negative-safety provider-matrix-vm-user-safety provider-matrix-vm-login-policy-safety provider-matrix-vm-kernel-module-safety provider-matrix-vm-host-locale provider-matrix-vm-time-sync provider-matrix-vm-mount provider-matrix-vm-swap provider-matrix-vm-systemd-timer provider-matrix-vm-systemd-unit provider-matrix-vm-service provider-matrix-vm-desktop-session provider-matrix-vm-failure-artifacts docker-server-build release-snapshot migrate migrate-compose install-agent-script docs-build docs-serve desktop-linux-prerequisites desktop-setup desktop-test desktop-dev desktop-build desktop-smoke desktop-package desktop-package-smoke desktop-release-manifest desktop-release-check desktop-flatpak desktop-flatpak-smoke desktop-flatpak-release-manifest desktop-flatpak-release-check \
	demo-fixtures demo-build demo-prepare demo-prepare-bootstrap demo-record demo-record-all

FUZZ_TIME ?= 30s
DOCKER_IMAGE ?= remotr-server
DOCKER_TAG ?= local
DESKTOP_DIR := $(CURDIR)/desktop
DESKTOP_FRONTEND_DIR := $(DESKTOP_DIR)/frontend
DESKTOP_PNPM ?= env COREPACK_HOME=$(DESKTOP_DIR)/.cache/corepack corepack pnpm@11.7.0
DESKTOP_WAILS_VERSION := v2.12.0
DESKTOP_VERSION ?= 0.0.0-dev
DESKTOP_PACKAGE_DIR := $(DESKTOP_DIR)/build/package
DESKTOP_DEB := $(DESKTOP_PACKAGE_DIR)/remotr-desktop_$(DESKTOP_VERSION)_amd64.deb
DESKTOP_RELEASE_MANIFEST := $(DESKTOP_PACKAGE_DIR)/release-manifest.json
DESKTOP_FLATPAK_PACKAGE_DIR := $(DESKTOP_DIR)/build/flatpak-package
DESKTOP_FLATPAK := $(DESKTOP_FLATPAK_PACKAGE_DIR)/remotr-desktop_$(DESKTOP_VERSION)_amd64.flatpak
DESKTOP_FLATPAK_RELEASE_MANIFEST := $(DESKTOP_FLATPAK_PACKAGE_DIR)/release-manifest.json
E2E_OPERATOR_CONFIG := $(CURDIR)/compose/runtime/operator/config.yaml

# Apply sql/schema.sql to production Postgres (Neon or any REMOTR_DATABASE_URL).
# Examples:
#   REMOTR_DATABASE_URL='postgres://...' make migrate
#   REMOTR_NEON_PROJECT=remotr-prod make migrate
#   REMOTR_NEON_PROJECT=remotr-prod REMOTR_FLEET=default make migrate
migrate:
	chmod +x scripts/migrate.sh
	./scripts/migrate.sh

# Apply schema to the local Compose Postgres (stack must be running).
migrate-compose:
	docker compose -f compose/docker-compose.yml exec -T postgres \
		psql -U remotr -d remotr -v ON_ERROR_STOP=1 -f - < sql/schema.sql

test: test-fuzz-seeds release-catalog-check
	go test -mod=vendor ./...

.PHONY: release-catalog-check
release-catalog-check:
	go run -mod=vendor ./internal/releasecatalog/cmd/generate -source test/qualification/ubuntu-pro.yaml -output internal/releasecatalog/generated_ubuntu_pro.go -check
	go run -mod=vendor ./internal/releasecatalog/cmd/generate-releases -source internal/releasecatalog/releases.yaml -output internal/releasecatalog/generated_releases.go -check

# Ordinary test runs execute every committed fuzz seed corpus by discovered
# target name before the repository suite. FUZZ_PACKAGES limits this to the
# affected package(s), e.g. `make test-fuzz-seeds FUZZ_PACKAGES=./internal/models`.
test-fuzz-seeds:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh --seed-corpora $(FUZZ_PACKAGES)

benchmark-capability-variants:
	go test -mod=vendor ./internal/server -run '^TestCompiledArtifactVariantsRemainSchemaBounded$$' -bench '^BenchmarkCapabilityVariantSelection400Endpoints$$' -benchmem -count=1
	go test -mod=vendor ./internal/store/postgres -run '^$$' -bench '^BenchmarkCompiledArtifactVariantsDatabaseBoundedBySchema$$' -benchmem -count=1

# Ten repeated samples on a controlled runner. Postgres must be the disposable
# Compose database or another explicitly approved benchmark database.
benchmark-foundation-controlled:
	@test -n "$(REMOTR_BENCH_DATABASE_URL)" || { echo 'REMOTR_BENCH_DATABASE_URL is required' >&2; exit 2; }
	go test -mod=vendor ./internal/store/postgres -run '^$$' -bench '^(BenchmarkPostgres|BenchmarkChangeControl)' -benchmem -count=10
	go test -mod=vendor ./internal/agent/engine -run '^$$' -bench '^BenchmarkAgentFullCycle' -benchmem -count=10

performance-budget-lint:
	go run -mod=vendor ./scripts/performance-budget-lint.go test/performance/budgets.json

performance-benchmark-gate:
	@test -n "$(BENCHMARK_FILE)" || { echo 'BENCHMARK_FILE is required' >&2; exit 2; }
	go run -mod=vendor ./scripts/performance-benchmark-gate.go test/performance/budgets.json "$(BENCHMARK_FILE)"

mutation-policy-lint:
	go run -mod=vendor ./scripts/mutation-survivor-baseline-lint.go
	$(MAKE) performance-budget-lint

mutation-capability-selection:
	chmod +x scripts/mutation-capability-selection.sh
	./scripts/mutation-capability-selection.sh

mutation-high-gate:
	chmod +x scripts/mutation-high-gate.sh
	./scripts/mutation-high-gate.sh

mutation-ubuntu-qualification:
	chmod +x scripts/mutation-high-gate.sh
	MUTATION_TARGET_FILE=test/mutation/ubuntu-qualification-targets.txt \
		MUTATION_EVIDENCE_FILE=artifacts/mutation/ubuntu-qualification-high-gate.json \
		./scripts/mutation-high-gate.sh

mutation-ubuntu-pro-management:
	chmod +x scripts/mutation-high-gate.sh
	MUTATION_TARGET_FILE=test/mutation/ubuntu-pro-management-targets.txt \
		MUTATION_EVIDENCE_FILE=artifacts/mutation/ubuntu-pro-management-high-gate.json \
		./scripts/mutation-high-gate.sh

ubuntu-2404-applicator-qualification-audit:
	@go run -mod=vendor ./scripts/ubuntu-qualification-audit.go

mutation-comprehensive:
	chmod +x scripts/mutation-comprehensive.sh scripts/mutation-high-gate.sh
	./scripts/mutation-comprehensive.sh

# Remotr Desktop is a nested module with a separately locked frontend. The
# package manifest pins pnpm, the lockfile pins JavaScript dependencies, and
# desktop/go.mod pins the Wails version used by these go run invocations.
desktop-setup:
	cd $(DESKTOP_FRONTEND_DIR) && $(DESKTOP_PNPM) install --frozen-lockfile
	cd $(DESKTOP_DIR) && go mod download

desktop-linux-prerequisites:
	./scripts/desktop-linux-prerequisites.sh --check

desktop-test: desktop-linux-prerequisites desktop-setup
	cd $(DESKTOP_DIR) && go test ./...
	@tags="$$(./scripts/desktop-linux-prerequisites.sh --wails-tags)"; \
		cd $(DESKTOP_DIR) && go test -tags "dev $$tags" -run '^TestDevelopmentApplicationAssetPolicyAllowsViteStylesOnly$$' ./...
	cd $(DESKTOP_FRONTEND_DIR) && $(DESKTOP_PNPM) test

desktop-dev: desktop-linux-prerequisites desktop-setup
	@tags="$$(./scripts/desktop-linux-prerequisites.sh --wails-tags)"; \
		cd $(DESKTOP_DIR) && go run github.com/wailsapp/wails/v2/cmd/wails@$(DESKTOP_WAILS_VERSION) dev -tags "$$tags"

desktop-build: desktop-linux-prerequisites desktop-setup
	@tags="$$(./scripts/desktop-linux-prerequisites.sh --wails-tags)"; \
		cd $(DESKTOP_DIR) && go run github.com/wailsapp/wails/v2/cmd/wails@$(DESKTOP_WAILS_VERSION) build -clean -tags "$$tags" -ldflags "-X main.version=$(DESKTOP_VERSION)"

desktop-smoke: desktop-build
	./scripts/desktop-native-smoke.sh --binary "$(DESKTOP_DIR)/build/bin/remotr-desktop" --version "$(DESKTOP_VERSION)"

desktop-package: desktop-build
	./scripts/desktop-package-deb.sh --binary "$(DESKTOP_DIR)/build/bin/remotr-desktop" --version "$(DESKTOP_VERSION)" --architecture amd64 --output "$(DESKTOP_DEB)"

desktop-package-smoke: desktop-package
	./scripts/desktop-package-smoke.sh --package "$(DESKTOP_DEB)" --version "$(DESKTOP_VERSION)" --native-smoke "$(CURDIR)/scripts/desktop-native-smoke.sh"

desktop-release-manifest: desktop-package-smoke
	python3 ./scripts/desktop-release-manifest.py generate --targets "$(DESKTOP_DIR)/build/linux/package-targets.json" --artifact "$(DESKTOP_DEB)" --version "$(DESKTOP_VERSION)" --os linux --architecture amd64 --format deb --output "$(DESKTOP_RELEASE_MANIFEST)"

desktop-release-check: desktop-release-manifest
	python3 ./scripts/desktop-release-manifest.py check --targets "$(DESKTOP_DIR)/build/linux/package-targets.json" --manifest "$(DESKTOP_RELEASE_MANIFEST)" --artifact-dir "$(DESKTOP_PACKAGE_DIR)"

desktop-flatpak: desktop-build
	./scripts/desktop-package-flatpak.sh --binary "$(DESKTOP_DIR)/build/bin/remotr-desktop" --version "$(DESKTOP_VERSION)" --architecture amd64 --output "$(DESKTOP_FLATPAK)"

desktop-flatpak-smoke: desktop-flatpak
	./scripts/desktop-flatpak-smoke.sh --package "$(DESKTOP_FLATPAK)" --version "$(DESKTOP_VERSION)"

desktop-flatpak-release-manifest: desktop-flatpak-smoke
	python3 ./scripts/desktop-release-manifest.py generate --targets "$(DESKTOP_DIR)/build/linux/package-targets.json" --artifact "$(DESKTOP_FLATPAK)" --version "$(DESKTOP_VERSION)" --os linux --architecture amd64 --format flatpak --output "$(DESKTOP_FLATPAK_RELEASE_MANIFEST)"

desktop-flatpak-release-check: desktop-flatpak-release-manifest
	python3 ./scripts/desktop-release-manifest.py check --targets "$(DESKTOP_DIR)/build/linux/package-targets.json" --manifest "$(DESKTOP_FLATPAK_RELEASE_MANIFEST)" --artifact-dir "$(DESKTOP_FLATPAK_PACKAGE_DIR)"

gosec:
	@command -v gosec >/dev/null 2>&1 || { echo "install: go install github.com/securego/gosec/v2/cmd/gosec@latest"; exit 1; }
	gosec -exclude-dir=vendor -exclude-generated -tests=false \
		--exclude-rules='internal/store/postgres/db/.*:G101' ./...

fuzz-short:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh 10s $(FUZZ_PACKAGES)

fuzz:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh $(FUZZ_TIME) $(FUZZ_PACKAGES)

vendor:
	go mod vendor

docker-server-build:
	docker build -f docker/remotr-server/Dockerfile -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

release-snapshot:
	@command -v goreleaser >/dev/null 2>&1 || { echo "install: https://goreleaser.com/install/"; exit 1; }
	goreleaser release --snapshot --clean

install-agent-script:
	chmod +x scripts/install-agent.sh

docs-build:
	chmod +x scripts/build-docs-site.sh
	./scripts/build-docs-site.sh

docs-serve:
	mkdocs serve

compose-up:
	chmod +x compose/scripts/gen-certs.sh compose/scripts/seed-compose-registry.sh compose/scripts/agent-entrypoint.sh
	docker compose -f compose/docker-compose.yml up -d --build --wait --remove-orphans

compose-down:
	docker compose -f compose/docker-compose.yml down -v
	@docker run --rm -v "$(CURDIR)/compose/runtime:/runtime" alpine:3.20 \
		sh -c 'rm -rf /runtime/agent-debian /runtime/agent-arch /runtime/enroll-tokens' 2>/dev/null || true

# Stack mirrors production: operator bootstrap, enrollment tokens, agent CSR enroll, mTLS sync.
test-e2e: compose-down compose-up
	@chmod 644 compose/runtime/certs/*.key 2>/dev/null || true
	@chmod 644 compose/runtime/bootstrap.token 2>/dev/null || true
	@for c in compose-agent-debian-1 compose-agent-arch-1; do \
		docker exec $$c sh -c 'chmod a+rx /var/lib/remotr && chmod a+r /var/lib/remotr/*' 2>/dev/null || true; \
	done
	REMOTR_CONFIG=$(E2E_OPERATOR_CONFIG) go test -mod=vendor -tags=e2e ./test/e2e/... -count=1 -v

test-e2e-quick:
	@chmod 644 compose/runtime/certs/*.key 2>/dev/null || true
	@chmod 644 compose/runtime/bootstrap.token 2>/dev/null || true
	@for c in compose-agent-debian-1 compose-agent-arch-1; do \
		docker exec $$c sh -c 'chmod a+rx /var/lib/remotr && chmod a+r /var/lib/remotr/*' 2>/dev/null || true; \
	done
	REMOTR_CONFIG=$(E2E_OPERATOR_CONFIG) go test -mod=vendor -tags=e2e ./test/e2e/... -count=1 -v

# Run only enroll flow (skips until POST /v1/enroll exists on the server).
test-e2e-enroll: compose-up
	REMOTR_CONFIG=$(E2E_OPERATOR_CONFIG) go test -mod=vendor -tags=e2e ./test/e2e/... -run TestEnroll -count=1 -v

# Requires explicit REMOTR_LOAD_* disposable-environment settings and --allow-load.
load-once:
	go run -mod=vendor ./cmd/remotr-load --allow-load

# Requires explicit REMOTR_LOAD_* disposable-environment settings. One warm-up
# artifact wave is followed by one unchanged wave at the default 30s interval.
load-steady-400:
	@go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 400 --concurrency 400 --steady-cycles 1

# Future-scale comparison only: this is headroom evidence, not a supported
# fleet-size promise. It retains the same default 30-second poll interval.
load-steady-4000:
	@go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 4000 --concurrency 4000 --steady-cycles 1

# Repeated authenticated Sync observations against the same 400 enrolled
# endpoints. Growth budgets come from the versioned assurance policy.
soak-smoke-400:
	@go run -mod=vendor ./cmd/remotr-load --allow-load --scenario soak --compose-file compose/docker-compose.yml --endpoints 400 --concurrency 400 --steady-cycles 2 --poll-interval 0

soak-medium-400:
	@go run -mod=vendor ./cmd/remotr-load --allow-load --scenario soak --compose-file compose/docker-compose.yml --endpoints 400 --concurrency 400 --steady-cycles 20 --poll-interval 30s

soak-long-400:
	@go run -mod=vendor ./cmd/remotr-load --allow-load --scenario soak --compose-file compose/docker-compose.yml --endpoints 400 --concurrency 400 --steady-cycles 120 --poll-interval 30s

# Requires explicit REMOTR_LOAD_* disposable-environment settings. Forces each
# endpoint client to establish fresh TLS connections for coordinated reconnects.
load-startup-reconnect-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --scenario startup-reconnect --endpoints 400 --concurrency 400

# Requires explicit REMOTR_LOAD_* disposable-environment settings. Restores the
# previous global release ref after exercising fleet fan-out and one override.
load-release-fanout-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --scenario release-fanout --endpoints 400 --concurrency 400

# Exercises current persisted Sync telemetry with bounded synthetic reports.
load-telemetry-heavy-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --scenario telemetry-heavy --endpoints 400 --concurrency 400

# Exercises five authenticated capability populations and requires the cache
# to grow only by the two declared schemas for each of two controlled Releases.
load-capability-mixed-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --scenario capability-mixed --endpoints 400 --concurrency 400

# Requires explicit REMOTR_LOAD_* disposable-environment settings. The command
# pauses and unpauses only the named local Compose service after --allow-faults.
load-server-recovery-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --allow-faults --scenario outage-recovery --compose-file compose/docker-compose.yml --fault-service remotr-server --request-timeout 2s --endpoints 400 --concurrency 400

load-postgres-recovery-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --allow-faults --scenario outage-recovery --compose-file compose/docker-compose.yml --fault-service postgres --request-timeout 2s --endpoints 400 --concurrency 400

# Reuses the agent polling policy's startup, stable-success, and transient
# backoff delays. Its 30-second stable-success interval is intentional.
load-policy-shaped-recovery-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --allow-faults --scenario policy-shaped-outage-recovery --compose-file compose/docker-compose.yml --fault-service remotr-server --request-timeout 2s --endpoints 400 --concurrency 400

# Recreates the local disposable server with a one-request admission limit,
# runs the typed-overload workload, then restores the prior Compose values.
load-overload-400:
	@set -e; \
		original_max="$${REMOTR_SYNC_MAX_CONCURRENT:-0}"; \
		original_retry="$${REMOTR_SYNC_RETRY_AFTER:-5s}"; \
		restore() { REMOTR_SYNC_MAX_CONCURRENT="$$original_max" REMOTR_SYNC_RETRY_AFTER="$$original_retry" docker compose -f compose/docker-compose.yml up -d --force-recreate --wait remotr-server >/dev/null; }; \
		trap restore EXIT; \
		REMOTR_SYNC_MAX_CONCURRENT=1 REMOTR_SYNC_RETRY_AFTER=1s docker compose -f compose/docker-compose.yml up -d --force-recreate --wait remotr-server; \
		go run -mod=vendor ./cmd/remotr-load --allow-load --scenario overload --endpoints 400 --concurrency 400

provider-matrix-containers:
	chmod +x scripts/provider-matrix-containers.sh
	./scripts/provider-matrix-containers.sh

provider-matrix-apt-debian-12:
	chmod +x scripts/provider-matrix-apt-container.sh
	./scripts/provider-matrix-apt-container.sh debian-12 debian 12

provider-matrix-apt-ubuntu-24-04:
	chmod +x scripts/provider-matrix-apt-container.sh
	./scripts/provider-matrix-apt-container.sh ubuntu-24.04 ubuntu 24.04

provider-matrix-apt-ubuntu-26-04:
	chmod +x scripts/provider-matrix-apt-container.sh
	./scripts/provider-matrix-apt-container.sh ubuntu-26.04 ubuntu 26.04

provider-matrix-apt-repository-debian-12:
	chmod +x scripts/provider-matrix-apt-repository-container.sh
	./scripts/provider-matrix-apt-repository-container.sh debian-12 debian 12

provider-matrix-apt-repository-ubuntu-24-04:
	chmod +x scripts/provider-matrix-apt-repository-container.sh
	./scripts/provider-matrix-apt-repository-container.sh ubuntu-24.04 ubuntu 24.04

provider-matrix-apt-repository-ubuntu-26-04:
	chmod +x scripts/provider-matrix-apt-repository-container.sh
	./scripts/provider-matrix-apt-repository-container.sh ubuntu-26.04 ubuntu 26.04

provider-matrix-pacman-arch-2026-07-06:
	chmod +x scripts/provider-matrix-pacman-container.sh
	./scripts/provider-matrix-pacman-container.sh

provider-matrix-pacman-repository-arch-2026-07-06:
	chmod +x scripts/provider-matrix-pacman-repository-container.sh
	./scripts/provider-matrix-pacman-repository-container.sh

provider-matrix-aur-arch-2026-07-06:
	chmod +x scripts/provider-matrix-aur-container.sh test/provider-matrix/fixtures/core-packages/aur/yay-controlled-fixture
	./scripts/provider-matrix-aur-container.sh

provider-package-fixtures:
	chmod +x scripts/verify-package-provider-fixtures.sh
	./scripts/verify-package-provider-fixtures.sh

provider-package-fixtures-reproducible:
	chmod +x scripts/generate-package-provider-fixtures.sh scripts/verify-package-provider-fixtures.sh
	./scripts/verify-package-provider-fixtures.sh --reproducible

provider-matrix-systemd-timer:
	go test -mod=vendor -tags=providerintegration ./internal/applicators/endpointschedules/systemdtimer -run '^TestApplicatorUsesRealSystemdAnalyzeVerification$$' -count=1 -v

provider-matrix-systemd-unit:
	go test -mod=vendor -tags=providerintegration ./internal/applicators/systemdunits -run '^TestApplicatorUsesRealSystemdAnalyzeVerification$$' -count=1 -v

provider-matrix-vm-up:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh up

provider-matrix-vm-restore:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh restore

provider-matrix-vm-destroy:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh destroy

provider-matrix-vm-lifecycle:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh lifecycle

provider-matrix-vm-network-recovery:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/network-recovery.sh
	./test/vagrant/harness.sh network-recovery

provider-matrix-vm-system-safety:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/system-safety.sh
	./test/vagrant/harness.sh system-safety

provider-matrix-vm-negative-safety:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/negative-safety.sh
	./test/vagrant/harness.sh negative-safety

provider-matrix-vm-user-safety:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh user-safety

provider-matrix-vm-login-policy-safety:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh login-policy-safety

provider-matrix-vm-kernel-module-safety:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh kernel-module-safety

provider-matrix-vm-host-locale:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh host-locale

provider-matrix-vm-time-sync:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh time-sync

provider-matrix-vm-mount:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh mount

provider-matrix-vm-swap:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh swap

provider-matrix-vm-systemd-timer:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh systemd-timer

provider-matrix-vm-systemd-unit:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh systemd-unit

provider-matrix-vm-service:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh service

provider-matrix-vm-desktop-session:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/desktop-session.sh
	./test/vagrant/harness.sh desktop-session

provider-matrix-vm-failure-artifacts:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/failure-artifacts.sh
	./test/vagrant/harness.sh failure-artifacts

provider-matrix-vm-core-delivery-ubuntu-26-04:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh core-delivery-ubuntu-26-04

provider-matrix-vm-ubuntu-pro-negative-identities:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh ubuntu-pro-negative-identities

provider-matrix-vm-ubuntu-pro-secret-canary:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh ubuntu-pro-secret-canary

provider-matrix-vm-ubuntu-pro-global-secret-canary:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh ubuntu-pro-secret-canary

provider-matrix-vm-ubuntu-pro-%:
	chmod +x test/vagrant/harness.sh
	./test/vagrant/harness.sh ubuntu-pro-selector $*

# --- Demo mode (REMOTR_DEMO) and VHS recordings for docs ---
# REMOTR_DEMO is set only by these targets (never in .tape files) so recordings stay clean.
DEMO_DIR := $(CURDIR)/demo
DEMO_ENV := REMOTR_DEMO=1 \
	REMOTR_DEMO_FIXTURES=$(DEMO_DIR)/fixtures/http \
	REMOTR_CONFIG=$(DEMO_DIR)/record/config/config.yaml \
	REMOTR_OPERATOR_STATE_DIR=$(DEMO_DIR)/record/state \
	REMOTR_SERVER_URL=https://demo.remotr.example:8443 \
	REMOTR_FLEET=engineering

demo-fixtures:
	chmod +x demo/scripts/gen-demo-certs.sh demo/scripts/seed-record-state.sh demo/scripts/seed-bootstrap-state.sh
	./demo/scripts/gen-demo-certs.sh

demo-build:
	go build -mod=vendor -o bin/remotr ./cmd/remotr

demo-prepare: demo-build demo-fixtures
	chmod +x demo/scripts/seed-record-state.sh
	./demo/scripts/seed-record-state.sh

demo-prepare-bootstrap: demo-build demo-fixtures
	chmod +x demo/scripts/seed-bootstrap-state.sh
	./demo/scripts/seed-bootstrap-state.sh

# Record one tape: make demo-record TAPE=init
demo-record: demo-prepare
	@command -v vhs >/dev/null 2>&1 || { echo "install: https://github.com/charmbracelet/vhs#installation"; exit 1; }
	@test -n "$(TAPE)" || { echo "usage: make demo-record TAPE=init  (tape name without .tape)"; exit 1; }
	@mkdir -p $(DEMO_DIR)/assets
	@sed 's|@REPO@|$(CURDIR)|g' $(DEMO_DIR)/tapes/$(TAPE).tape > $(DEMO_DIR)/tapes/.record.tape
	@$(DEMO_ENV) vhs $(DEMO_DIR)/tapes/.record.tape
	@rm -f $(DEMO_DIR)/tapes/.record.tape

demo-record-all: demo-prepare
	@command -v vhs >/dev/null 2>&1 || { echo "install: https://github.com/charmbracelet/vhs#installation"; exit 1; }
	@mkdir -p $(DEMO_DIR)/assets
	@for t in init bootstrap enroll-token endpoint-list endpoint-show deployment git-sync config-validate doctor; do \
		echo "==> recording $$t"; \
		sed 's|@REPO@|$(CURDIR)|g' $(DEMO_DIR)/tapes/$$t.tape > $(DEMO_DIR)/tapes/.record.tape; \
		$(DEMO_ENV) vhs $(DEMO_DIR)/tapes/.record.tape || exit 1; \
	done
	@rm -f $(DEMO_DIR)/tapes/.record.tape
	@echo "GIFs written to $(DEMO_DIR)/assets/"
