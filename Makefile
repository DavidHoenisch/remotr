.PHONY: test vendor fuzz fuzz-short gosec compose-up compose-down test-e2e test-e2e-quick test-e2e-enroll docker-server-build release-snapshot migrate migrate-compose install-agent-script \
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

test:
	go test -mod=vendor ./...

gosec:
	@command -v gosec >/dev/null 2>&1 || { echo "install: go install github.com/securego/gosec/v2/cmd/gosec@latest"; exit 1; }
	gosec -exclude-dir=vendor -exclude-generated -tests=false \
		--exclude-rules='internal/store/postgres/db/.*:G101' ./...

fuzz-short:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh 10s

fuzz:
	chmod +x scripts/fuzz-all.sh
	./scripts/fuzz-all.sh $(FUZZ_TIME)

vendor:
	go mod vendor

docker-server-build:
	docker build -f docker/remotr-server/Dockerfile -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

release-snapshot:
	@command -v goreleaser >/dev/null 2>&1 || { echo "install: https://goreleaser.com/install/"; exit 1; }
	goreleaser release --snapshot --clean

install-agent-script:
	chmod +x scripts/install-agent.sh

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
	@for t in init bootstrap enroll-token endpoint-list endpoint-show deployment git-sync config-validate; do \
		echo "==> recording $$t"; \
		sed 's|@REPO@|$(CURDIR)|g' $(DEMO_DIR)/tapes/$$t.tape > $(DEMO_DIR)/tapes/.record.tape; \
		$(DEMO_ENV) vhs $(DEMO_DIR)/tapes/.record.tape || exit 1; \
	done
	@rm -f $(DEMO_DIR)/tapes/.record.tape
	@echo "GIFs written to $(DEMO_DIR)/assets/"
