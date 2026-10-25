# nullifier — 开发任务入口
#
# Windows:  在 Git for Windows 的 bash 环境下执行 `mingw32-make <target>`
#           (PATH 中需要有 Git 的 usr/bin 和 mingw32-make)
# Linux/macOS: 直接 `make <target>`
#
# 本文件必须保持 LF 换行，否则 make 会报 "missing separator"。
# `make help` 的输出刻意使用纯 ASCII：Windows 终端的 locale 常常不是 UTF-8，
# 中文提示会变成乱码。中文说明请写在注释里，不要放进 ## 描述。

SHELL := /bin/bash
.SHELLFLAGS := -eo pipefail -c

GO ?= go
PNPM ?= pnpm

# SQLite 驱动依赖 cgo，Windows 上必须显式开启，否则 go test 会报
# "Binary was compiled with 'CGO_ENABLED=0'"。
ifeq ($(OS),Windows_NT)
CGO_ENABLED ?= 1
export CGO_ENABLED
endif

.DEFAULT_GOAL := help

.PHONY: help \
	deps tidy build run cli loadtest \
	test test-short cover pg-test \
	vet fmt fmt-check \
	web-install web-dev web-build web-test web-lint web-typecheck web-format web-format-check \
	web-wasm web-proving-key web-eligibility-key eligibility-list \
	browser-acceptance \
	check ci clean

help: ## Show available targets
	@printf "nullifier targets:\n"
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| sed -E 's/^([a-zA-Z_-]+):.*## (.*)$$/\1\t\2/' \
		| awk -F'\t' '{printf "  %-18s %s\n", $$1, $$2}'

# ---------- 依赖 ----------

deps: ## Download Go dependencies
	$(GO) mod download

tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

# ---------- 构建与运行 ----------

build: ## Build every package
	$(GO) build ./...

run: ## Start the HTTP server (SQLite file anonvote.db)
	$(GO) run ./cmd/server

cli: ## Run the CLI (make cli ARGS="help")
	$(GO) run ./cmd/cli $(ARGS)

loadtest: ## Run the concurrent voting load test (make loadtest ARGS="-votes 200")
	$(GO) run ./cmd/loadtest $(ARGS)

# ---------- 测试 ----------

test: ## Run every test
	$(GO) test ./...

test-short: ## Run tests, skipping container-backed integration tests
	$(GO) test -short ./...

cover: ## Coverage gate (per-package floors in scripts/coverage-thresholds.txt)
	bash ./scripts/check-coverage.sh $(or $(THRESHOLD),80) coverage.out

pg-test: ## Run the PostgreSQL integration tests against a local compose database
	@echo "starting postgres (docker compose) ..."; \
	docker compose up -d --wait postgres; \
	echo "running the postgres integration tests ..."; \
	status=0; \
	ANONVOTE_TEST_PG_HOST=127.0.0.1 \
	ANONVOTE_TEST_PG_PORT=5432 \
	ANONVOTE_TEST_PG_USER=test \
	ANONVOTE_TEST_PG_PASSWORD=test \
	ANONVOTE_TEST_PG_DATABASE=anonvote \
	$(GO) test -count=1 -v ./tests/integration/ -run Postgres || status=$$?; \
	echo "stopping postgres ..."; \
	docker compose down -v || true; \
	exit $$status

# ---------- 静态检查 ----------

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Format Go sources
	gofmt -w ./cmd ./internal ./tests

fmt-check: ## Fail when Go sources are not gofmt-clean
	@out="$$(gofmt -l ./cmd ./internal ./tests)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt failed, these files need formatting:"; \
		echo "$$out"; \
		exit 1; \
	fi

# ---------- 前端 ----------

web-install: ## Install frontend dependencies
	$(PNPM) --dir web install

web-dev: ## Start the frontend dev server on :3000
	$(PNPM) --dir web run dev

web-build: ## Build the frontend
	$(PNPM) --dir web run build

web-test: ## Run frontend tests (vitest)
	$(PNPM) --dir web run test

web-lint: ## Run frontend ESLint
	$(PNPM) --dir web run lint

web-typecheck: ## Run frontend type checking
	$(PNPM) --dir web exec tsc --noEmit

web-format: ## Format frontend sources
	$(PNPM) --dir web run format

web-format-check: ## Check frontend formatting
	$(PNPM) --dir web exec prettier --check .

web-wasm: ## Build the browser-side prover (WASM) into web/public
	@mkdir -p web/public
	GOOS=js GOARCH=wasm CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" \
		-o web/public/ballot-prover.wasm ./cmd/ballot-wasm
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js
	@echo "web/public/ballot-prover.wasm and web/public/wasm_exec.js are ready"

web-proving-key: ## Publish the proving key to web/public (make web-proving-key PK=<path>)
	@if [ -z "$(PK)" ]; then \
		echo "usage: make web-proving-key PK=<path to proving.key>"; \
		exit 2; \
	fi
	@mkdir -p web/public
	cp "$(PK)" web/public/proving.key
	@echo "web/public/proving.key is ready (never commit it)"

web-eligibility-key: ## Publish the eligibility proving key (make web-eligibility-key PK=<path>)
	@if [ -z "$(PK)" ]; then \
		echo "usage: make web-eligibility-key PK=<path to eligibility-proving.key>"; \
		exit 2; \
	fi
	@mkdir -p web/public
	cp "$(PK)" web/public/eligibility-proving.key
	@echo "web/public/eligibility-proving.key is ready (never commit it)"

eligibility-list: ## Build the offline eligibility-list tool
	$(GO) build -o eligibility-list ./cmd/eligibility-list

browser-acceptance: ## Prepare the browser acceptance environment (make browser-acceptance ARGS=setup|verify|stop)
	bash ./scripts/browser-acceptance.sh $(or $(ARGS),setup)

# ---------- 组合目标 ----------

check: vet fmt-check web-lint web-typecheck ## Fast local gate before committing

ci: test cover ## Reproduce the CI pipeline: go test ./... + coverage gate

clean: ## Remove build output and coverage files
	rm -f coverage*.out probe.out
	rm -rf web/dist
