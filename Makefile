# burnerpad-cli — the one canonical build-flags block (ARCHITECTURE.md §22.2).
# CI and goreleaser must use exactly these flags; drift here breaks reproducibility.

ifeq ($(origin VERSION), undefined)
override BUILD_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
else
override BUILD_VERSION := $(value VERSION)
endif
ifeq ($(origin COMMIT), undefined)
override BUILD_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
else
override BUILD_COMMIT := $(value COMMIT)
endif
ifeq ($(origin DATE), undefined)
override BUILD_DATE := $(shell git log -1 --format=%cd --date=format:%Y-%m-%d 2>/dev/null || echo unknown)
else
override BUILD_DATE := $(value DATE)
endif

.DEFAULT_GOAL := build

# Build metadata is frozen once, exported as data, and expanded by the recipe
# shell. Never interpolate it into recipe source: Git ref names may legally
# contain shell metacharacters. The validation target also keeps values safe
# for Go's own space-delimited -ldflags parser.
unexport VERSION COMMIT DATE
export BUILD_VERSION BUILD_COMMIT BUILD_DATE
override LDFLAGS = -s -w -buildid= -X main.version=$${BUILD_VERSION} -X main.commit=$${BUILD_COMMIT} -X main.date=$${BUILD_DATE}
override GOFLAGS = -trimpath -buildvcs=false

PREFIX  ?= /usr/local
DESTDIR ?=

.PHONY: build test lint fuzz vet clean install cross size validate-build-metadata

validate-build-metadata:
	@LC_ALL=C; export LC_ALL; \
	case "$$BUILD_VERSION" in \
	  ''|*[!A-Za-z0-9._+-]*) printf '%s\n' "invalid VERSION build metadata" >&2; exit 2 ;; \
	esac; \
	case "$$BUILD_COMMIT" in \
	  unknown) ;; \
	  ''|*[!0-9a-f]*) printf '%s\n' "invalid COMMIT build metadata" >&2; exit 2 ;; \
	esac; \
	case "$$BUILD_DATE" in \
	  unknown|[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;; \
	  *) printf '%s\n' "invalid DATE build metadata" >&2; exit 2 ;; \
	esac

build: validate-build-metadata
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o burnerpad ./cmd/burnerpad

test:
	go test ./...

test-short:
	go test -short ./...

vet:
	go vet ./...
	gofmt -l . | (! grep .)

lint: vet
	command -v staticcheck >/dev/null && staticcheck ./... || echo "staticcheck not installed; skipped"

fuzz:
	go test ./envelope -fuzz FuzzDecodeCanonical -fuzztime 60s
	go test ./internal/id -fuzz FuzzNormalizeID -fuzztime 30s
	go test ./internal/id -fuzz FuzzParseShareURL -fuzztime 30s

cross: validate-build-metadata
	@for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; out=dist/burnerpad_$${os}_$${arch}; \
	  [ $$os = windows ] && out=$$out.exe; \
	  echo "build $$t"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $$out ./cmd/burnerpad || exit 1; \
	done

size: cross
	./scripts/check-size.sh dist/*

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 burnerpad $(DESTDIR)$(PREFIX)/bin/burnerpad

clean:
	rm -rf burnerpad dist
