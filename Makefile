.PHONY: test test-fuzz-seeds vendor fuzz fuzz-short gosec compose-up compose-down test-e2e test-e2e-quick test-e2e-enroll load-once load-steady-400 load-steady-4000 load-startup-reconnect-400 load-release-fanout-400 load-telemetry-heavy-400 load-server-recovery-400 load-postgres-recovery-400 load-policy-shaped-recovery-400 load-overload-400 provider-matrix-containers provider-matrix-vm-up provider-matrix-vm-restore provider-matrix-vm-destroy provider-matrix-vm-lifecycle provider-matrix-vm-network-recovery provider-matrix-vm-system-safety provider-matrix-vm-negative-safety provider-matrix-vm-user-safety provider-matrix-vm-failure-artifacts docker-server-build release-snapshot migrate migrate-compose install-agent-script docs-build docs-serve \
	demo-fixtures demo-build demo-prepare demo-prepare-bootstrap demo-record demo-record-all

FUZZ_TIME ?= 30s
DOCKER_IMAGE ?= remotr-server
DOCKER_TAG ?= local

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

test: test-fuzz-seeds
	go test -mod=vendor ./...

# Ordinary test runs execute every committed fuzz seed corpus by discovered
# target name before the repository suite. FUZZ_PACKAGES limits this to the
# affected package(s), e.g. `make test-fuzz-seeds FUZZ_PACKAGES=./internal/models`.
test-fuzz-seeds:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh --seed-corpora $(FUZZ_PACKAGES)

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
	go test -mod=vendor -tags=e2e ./test/e2e/... -count=1 -v

test-e2e-quick:
	@chmod 644 compose/runtime/certs/*.key 2>/dev/null || true
	@chmod 644 compose/runtime/bootstrap.token 2>/dev/null || true
	@for c in compose-agent-debian-1 compose-agent-arch-1; do \
		docker exec $$c sh -c 'chmod a+rx /var/lib/remotr && chmod a+r /var/lib/remotr/*' 2>/dev/null || true; \
	done
	go test -mod=vendor -tags=e2e ./test/e2e/... -count=1 -v

# Run only enroll flow (skips until POST /v1/enroll exists on the server).
test-e2e-enroll: compose-up
	go test -mod=vendor -tags=e2e ./test/e2e/... -run TestEnroll -count=1 -v

# Requires explicit REMOTR_LOAD_* disposable-environment settings and --allow-load.
load-once:
	go run -mod=vendor ./cmd/remotr-load --allow-load

# Requires explicit REMOTR_LOAD_* disposable-environment settings. One warm-up
# artifact wave is followed by one unchanged wave at the default 30s interval.
load-steady-400:
	go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 400 --concurrency 400 --steady-cycles 1

# Future-scale comparison only: this is headroom evidence, not a supported
# fleet-size promise. It retains the same default 30-second poll interval.
load-steady-4000:
	go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 4000 --concurrency 4000 --steady-cycles 1

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

provider-matrix-vm-failure-artifacts:
	chmod +x test/vagrant/harness.sh test/vagrant/fixtures/failure-artifacts.sh
	./test/vagrant/harness.sh failure-artifacts

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
