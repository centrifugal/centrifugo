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

build:
	CGO_ENABLED=0 go build

.PHONY: pg-schemas

PG_SCHEMA_DIR      = internal/pgmapbroker/internal/sql
PG_SCHEMA_TEMPLATE = $(PG_SCHEMA_DIR)/schema.sql
PG_SCHEMA_JSONB    = $(PG_SCHEMA_DIR)/schema_jsonb.sql
PG_SCHEMA_BINARY   = $(PG_SCHEMA_DIR)/schema_binary.sql
PG_SCHEMA_ALL      = $(PG_SCHEMA_DIR)/schema_all.sql

pg-schemas: $(PG_SCHEMA_JSONB) $(PG_SCHEMA_BINARY) $(PG_SCHEMA_ALL)
	@echo "Generated $(PG_SCHEMA_JSONB), $(PG_SCHEMA_BINARY), and $(PG_SCHEMA_ALL)"

$(PG_SCHEMA_JSONB): $(PG_SCHEMA_TEMPLATE)
	@echo '-- Auto-generated from schema.sql — do not edit.' > $@
	@sed -n '/^-- Stream Table/,$$p' $< | sed -e 's/__DATA_TYPE__/JSONB/g' -e 's/__PREFIX__/cf_map_/g' >> $@

$(PG_SCHEMA_BINARY): $(PG_SCHEMA_TEMPLATE)
	@echo '-- Auto-generated from schema.sql — do not edit.' > $@
	@sed -n '/^-- Stream Table/,$$p' $< | sed -e 's/__DATA_TYPE__/BYTEA/g' -e 's/__PREFIX__/cf_binary_map_/g' >> $@

$(PG_SCHEMA_ALL): $(PG_SCHEMA_JSONB) $(PG_SCHEMA_BINARY)
	@echo '-- Auto-generated unified schema — do not edit.' > $@
	@echo '-- Apply this file for fresh installs. For upgrades, see migrations/.' >> $@
	@echo '' >> $@
	@cat $(PG_SCHEMA_JSONB) >> $@
	@echo '' >> $@
	@cat $(PG_SCHEMA_BINARY) >> $@
