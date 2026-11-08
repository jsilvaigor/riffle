.PHONY: help check test media dev build run stop restart logs shell diag backup release

VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")
IMAGE_NAME ?= registry.isilva.dev/riffle-server
# Server releases get their own tag namespace; the iOS app's tags stay untouched.
TAG_PREFIX := server-v
# patch, minor or major: `make release BUMP=minor`
BUMP ?= patch
# Where `make diag` points: `make diag URL=http://192.168.100.13:8770`. The token comes
# from TOKEN=, $RIFFLE_TOKEN, ~/.config/riffle/token, or "dev" for local runs.
URL ?= http://localhost:8770
TOKEN ?= $(or $(RIFFLE_TOKEN),$(shell cat ~/.config/riffle/token 2>/dev/null),dev)

help:
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-9s %s\n", $$1, $$2}'

check: ## gofmt, vet and a race build (the CLAUDE.md checks)
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go build -race -o /tmp/riffle .

test: check ## check + unit tests with the race detector
	go test -race ./...

# A few codecs the iOS client has to handle; skipped if already generated.
media: ## generate test videos in testdata/media
	@mkdir -p testdata/media
	ffmpeg -hide_banner -loglevel error -n -f lavfi -i testsrc2=duration=60:size=1280x720:rate=30 \
		-c:v libx264 -preset veryfast -crf 30 -g 60 testdata/media/h264.mkv || true
	ffmpeg -hide_banner -loglevel error -n -f lavfi -i testsrc2=duration=60:size=1920x1080:rate=24 \
		-c:v libx265 -preset ultrafast -crf 32 -tag:v hvc1 -x265-params log-level=error testdata/media/hevc.mp4 || true
	ffmpeg -hide_banner -loglevel error -n -f lavfi -i testsrc2=duration=60:size=640x480:rate=25 \
		-c:v wmv2 -b:v 1M testdata/media/wmv2.wmv || true
	ffmpeg -hide_banner -loglevel error -n -f lavfi -i testsrc2=duration=4:size=1080x1430:rate=30 \
		-c:v libx264 -g 250 testdata/media/short-portrait.mp4 || true
	@# A phone-style portrait clip: landscape pixels plus a 90° display rotation.
	ffmpeg -hide_banner -loglevel error -n -f lavfi -i testsrc2=duration=30:size=1920x1080:rate=30 \
		-c:v libx264 -preset veryfast -crf 30 -g 60 /tmp/riffle-portrait-raw.mp4 && \
		ffmpeg -hide_banner -loglevel error -n -display_rotation:v:0 -90 -i /tmp/riffle-portrait-raw.mp4 \
		-c copy testdata/media/portrait-phone.mp4 || true
	@rm -f /tmp/riffle-portrait-raw.mp4

dev: ## run the server natively against testdata/media (CPU decoding)
	RIFFLE_MEDIA=Test=$(CURDIR)/testdata/media RIFFLE_DATA=$(CURDIR)/data RIFFLE_TOKEN=$(TOKEN) \
		RIFFLE_HWACCEL=none go run -ldflags "-X main.version=$(VERSION)-dev" .

# Local container (docker-compose.yml): http://localhost:8770/api/health
build: ## build the local image
	docker compose build

run: ## start the local container
	docker compose up -d

stop: ## stop the local container
	docker compose down

restart: stop run

logs: ## follow the local container's logs
	docker compose logs -f

shell: ## open a shell in the local container
	docker compose exec riffle /bin/sh

diag: ## call /api/diagnostics (decode tests take a few seconds)
	@curl -sfS -H "Authorization: Bearer $(TOKEN)" "$(URL)/api/diagnostics" | \
		(python3 -m json.tool 2>/dev/null || cat)

# Resume positions are the only state that can't be rebuilt by a rescan.
backup: ## copy progress.json and library.json from the local container into backups/
	@ts=$$(date +%Y%m%d-%H%M%S); mkdir -p backups/$$ts && \
	docker compose cp riffle:/data/progress.json backups/$$ts/ && \
	docker compose cp riffle:/data/library.json backups/$$ts/ && \
	echo "backup: backups/$$ts"

# From a pushed, clean main: tests, bump VERSION, build linux/amd64 for Unraid, then
# push the commit, the server-vX.Y.Z tag and the :X.Y.Z and :latest images. Nothing is
# pushed until the image has built.
release: ## release a new version: make release [BUMP=minor]
	@echo "==> Checking that code is committed and pushed..."
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != main ]; then \
		echo "Error: release from main."; exit 1; \
	fi
	@git fetch origin --quiet 2>/dev/null || true
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: uncommitted changes. Commit and push first."; exit 1; \
	fi
	@if [ "$$(git rev-parse HEAD)" != "$$(git rev-parse origin/main)" ]; then \
		echo "Error: main differs from origin/main. Pull or push first."; exit 1; \
	fi
	@echo "==> Running tests..."
	@$(MAKE) --no-print-directory test
	@echo "==> Bumping VERSION ($(BUMP))..."
	@awk -F. -v b=$(BUMP) '{ if (b=="major") {$$1++; $$2=0; $$3=0} \
		else if (b=="minor") {$$2++; $$3=0} else {$$3++} } 1' OFS=. VERSION > VERSION.tmp && \
		mv VERSION.tmp VERSION
	@NEW_VERSION=$$(cat VERSION); \
	TAG="$(TAG_PREFIX)$$NEW_VERSION"; \
	if git rev-parse -q --verify "refs/tags/$$TAG" >/dev/null; then \
		git checkout VERSION; echo "Error: tag $$TAG already exists."; exit 1; \
	fi; \
	echo "==> Releasing $$NEW_VERSION (tag $$TAG)"; \
	git add VERSION && \
	git commit -q -m "chore(server): release $$NEW_VERSION" && \
	git tag -a "$$TAG" -m "Riffle Server $$NEW_VERSION" && \
	echo "==> Building docker image..." && \
	docker build --platform linux/amd64 \
		--build-arg VERSION=$$NEW_VERSION --build-arg COMMIT=$$(git rev-parse --short HEAD) \
		-t $(IMAGE_NAME):$$NEW_VERSION -t $(IMAGE_NAME):latest . || { \
		echo "Error: build failed. Undo with: git tag -d $$TAG && git reset --hard HEAD~1"; \
		exit 1; } ; \
	echo "==> Pushing commit, tag and image..." && \
	git push -q origin HEAD "$$TAG" && \
	docker push -q $(IMAGE_NAME):$$NEW_VERSION && \
	docker push -q $(IMAGE_NAME):latest && \
	echo "==> Released $$NEW_VERSION: $(IMAGE_NAME):$$NEW_VERSION"
