# uniproxy/Makefile — Docker-обёртки для сборки, тестирования и линтинга.
# Не требует локальной установки Go или golangci-lint.
#
# Использование:
#   make pull           — скачать необходимые Docker-образы (первый запуск)
#   make build          — компиляция
#   make test           — тесты с race detection
#   make test-coverage  — тесты с покрытием
#   make lint           — golangci-lint
#   make security       — проверка безопасности (gosec)
#   make audit          — проверка зависимостей на CVE (govulncheck)
#   make deadcode       — поиск мёртвого кода
#   make fmt            — goimports + gofmt
#   make helm-lint      — валидация Helm-чартов
#   make hadolint       — проверка Dockerfile
#   make check-all      — все проверки
#   make clean          — очистка кэшей

# --- Переменные ---
GO_VERSION     ?= 1.25
LINT_VERSION   ?= v2.8.0
IMAGE_REGISTRY ?= docker.io

# Docker volumes
CACHE_VOLUME      = uniproxy-go-cache
CACHE_VOLUME_LINT = uniproxy-go-cache-lint

# Docker-образы
GO_IMAGE      = $(IMAGE_REGISTRY)/golang:$(GO_VERSION)
LINT_IMAGE    = $(IMAGE_REGISTRY)/golangci/golangci-lint:$(LINT_VERSION)
HELM_IMAGE    = $(IMAGE_REGISTRY)/alpine/helm:latest
HADOLINT_IMAGE = $(IMAGE_REGISTRY)/hadolint/hadolint:latest

# Префикс имён контейнеров
CN = uniproxy

# Удаление предыдущего контейнера (если остался после прерывания)
define remove_old
	@docker rm -f $(CN)-$(1) 2>/dev/null || true
endef

# Общие флаги docker run
DOCKER_RUN = docker run --rm \
	-v $(PWD):/workspace \
	-v $(CACHE_VOLUME):/go \
	-w /workspace \
	-e GOFLAGS=-buildvcs=false

.PHONY: build test test-coverage lint fmt security audit deadcode helm-lint hadolint check-all pull clean help

## build: компиляция всех пакетов
build:
	$(call remove_old,build)
	$(DOCKER_RUN) --name $(CN)-build $(GO_IMAGE) go build ./...

## test: запуск тестов с race detection
test:
	$(call remove_old,test)
	$(DOCKER_RUN) --name $(CN)-test $(GO_IMAGE) go test -race -count=1 ./...

## test-coverage: тесты с отчётом о покрытии
test-coverage:
	$(call remove_old,coverage)
	$(DOCKER_RUN) --name $(CN)-coverage $(GO_IMAGE) \
		sh -c 'go test -race -count=1 -coverprofile=/tmp/coverage.out ./... && go tool cover -func=/tmp/coverage.out'

## lint: статический анализ (golangci-lint)
lint:
	$(call remove_old,lint)
	docker run --rm --name $(CN)-lint \
		-v $(PWD):/workspace \
		-v $(CACHE_VOLUME_LINT):/root/.cache \
		-w /workspace \
		-e GOFLAGS=-buildvcs=false \
		$(LINT_IMAGE) \
		golangci-lint run -v ./...

## security: проверка безопасности кода (gosec, через golangci-lint)
security:
	$(call remove_old,security)
	docker run --rm --name $(CN)-security \
		-v $(PWD):/workspace \
		-v $(CACHE_VOLUME_LINT):/root/.cache \
		-w /workspace \
		-e GOFLAGS=-buildvcs=false \
		$(LINT_IMAGE) \
		golangci-lint run -v --enable-only gosec ./...

## audit: проверка зависимостей на уязвимости (govulncheck)
audit:
	$(call remove_old,audit)
	$(DOCKER_RUN) --name $(CN)-audit $(GO_IMAGE) \
		sh -c 'go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...'

## deadcode: поиск мёртвого кода (deadcode)
deadcode:
	$(call remove_old,deadcode)
	$(DOCKER_RUN) --name $(CN)-deadcode $(GO_IMAGE) \
		sh -c 'go install golang.org/x/tools/cmd/deadcode@latest && deadcode -test ./...'

## fmt: форматирование кода (goimports + gofmt)
fmt:
	$(call remove_old,fmt)
	$(DOCKER_RUN) --name $(CN)-fmt $(GO_IMAGE) \
		sh -c 'go install golang.org/x/tools/cmd/goimports@latest && \
			goimports -w -local github.com/BigKAA/uniproxy . && \
			gofmt -w .'

## helm-lint: валидация Helm-чартов
helm-lint:
	docker run --rm \
		--entrypoint sh \
		-v $(PWD):/workspace \
		-w /workspace \
		$(HELM_IMAGE) \
		-c 'helm lint charts/uniproxy && helm lint deploy/helm/uniproxy'

## hadolint: проверка Dockerfile
hadolint:
	docker run --rm -i $(HADOLINT_IMAGE) < Dockerfile

## check-all: все проверки (lint + test + audit + deadcode + helm-lint + hadolint)
check-all: lint test audit deadcode helm-lint hadolint

## pull: скачать все необходимые Docker-образы
pull:
	docker pull $(GO_IMAGE)
	docker pull $(LINT_IMAGE)
	docker pull $(HELM_IMAGE)
	docker pull $(HADOLINT_IMAGE)
	@echo ""
	@echo "Все образы скачаны:"
	@echo "  $(GO_IMAGE)       — build, test, fmt, audit, deadcode"
	@echo "  $(LINT_IMAGE) — lint, security"
	@echo "  $(HELM_IMAGE)     — helm-lint"
	@echo "  $(HADOLINT_IMAGE) — hadolint"

## clean: удаление Docker volumes с кэшем
clean:
	docker volume rm $(CACHE_VOLUME) 2>/dev/null || true
	docker volume rm $(CACHE_VOLUME_LINT) 2>/dev/null || true

## help: список целей
help:
	@echo "Цели:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
