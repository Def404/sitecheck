BINARY   := sitecheck
MODULE   := sitecheck
GO       ?= go
VERSION  ?= 0.4.2
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w \
            -X main.version=$(VERSION) \
            -X main.commit=$(COMMIT) \
            -X main.buildDate=$(DATE)

# Каталог main-пакета (модуль — в корне репозитория)
PKG := ./sitecheck

# Целевые платформы кросс-компиляции (platform → suffix).
# windows-arm64 закомментирован: Go кросс-компилирует, но без CGo есть проблемы с exec
PLATFORMS := \
    linux-amd64 \
    linux-arm64 \
    darwin-amd64 \
    darwin-arm64 \
    windows-amd64

.PHONY: all build test vet lint fmt fmt-check tidy clean cross dist help

## all: сборка + тесты + vet (базовый smoke)
all: build test vet

## build: собрать текущий бинарник в dist/
build:
	@mkdir -p dist
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY) $(PKG)
	@echo "Built: dist/$(BINARY)"

## test: прогон всех тестов
test:
	$(GO) test -race -count=1 $(PKG)/...

## vet: go vet
vet:
	$(GO) vet $(PKG)/...

## lint: golangci-lint (если установлен)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run $(PKG)/...

## fmt: gofmt (в каталоге main-пакета)
fmt:
	gofmt -w $(PKG)

## fmt-check: проверка без правки (для CI)
fmt-check:
	@bad=$$(gofmt -l $(PKG)); \
	if [ -n "$$bad" ]; then \
		echo "gofmt: needs formatting:"; echo "$$bad"; \
		exit 1; \
	else \
		echo "gofmt: ok"; \
	fi

## tidy: go mod tidy
tidy:
	$(GO) mod tidy

## cross: кросс-компиляция всех PLATFORMS в dist/
cross: clean
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		goos=$${p%-*}; goarch=$${p#*-}; \
		suffix=""; \
		[ "$$goos" = "windows" ] && suffix=".exe"; \
		out="dist/$(BINARY)-$$p$$suffix"; \
		echo "Building $$out"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch \
			$(GO) build -trimpath -ldflags "$(LDFLAGS)" \
			-o $$out $(PKG) || exit 1; \
	done
	@echo "--- dist/ ---"
	@ls -lh dist/

## dist: алиас для cross
dist: cross

## clean: убрать dist/
clean:
	rm -rf dist

## help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
