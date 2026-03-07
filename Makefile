VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')
ITERATION := $(shell git describe --tags --long | awk -F "-" '{print $$2}')
TESTFOLDERS := $(shell go list ./... | grep -v /misc/)

all: test

test:
	go test -count=1 -v $(TESTFOLDERS) -cover -race

test-integration:
	go test -count=1 -v $(TESTFOLDERS) -cover -race --tags=integration

generate:
	bash misc/scripts/generate.sh
	go generate ./...
	go run internal/cli/configdoc/main.go

web:
	./misc/scripts/update_web.sh

swagger-web:
	make generate
	./misc/scripts/update_swagger_web.sh

package:
	./misc/scripts/package.sh $(VERSION) $(ITERATION)

sync-websocket:
	rsync -av --exclude '*/' ../centrifuge/internal/websocket/ internal/websocket/

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/buster PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/bullseye PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/bookworm PACKAGES/*.deb

	package_cloud push FZambia/centrifugo/ubuntu/focal PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/jammy PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/noble PACKAGES/*.deb

packagecloud-rpm:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/el/7 PACKAGES/*.rpm

deps:
	go mod tidy

local-deps:
	go mod tidy
	go mod download
	go mod vendor

GOLANGCI_LINT_VERSION := $(shell cat .golangci-lint-version)

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found. Run 'make install-lint' to install $(GOLANGCI_LINT_VERSION)."; \
		exit 1; \
	fi
	@INSTALLED=$$(golangci-lint version --short 2>/dev/null); \
	EXPECTED=$$(echo $(GOLANGCI_LINT_VERSION) | sed 's/^v//'); \
	if [ "$${INSTALLED%%.*}" != "$${EXPECTED%%.*}" ] || [ "$${INSTALLED#*.}" != "$${EXPECTED#*.}" ]; then \
		echo "golangci-lint version mismatch: installed $${INSTALLED}, expected $${EXPECTED}. Run 'make install-lint'."; \
		exit 1; \
	fi
	golangci-lint run --timeout 10m0s --verbose

install-lint:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

build:
	CGO_ENABLED=0 go build
