export GO111MODULE=on

VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')
ITERATION := $(shell git describe --tags --long | awk -F "-" '{print $$2}')
TESTFOLDERS := $(shell go list ./... | grep -v /vendor/ | grep -v /misc/)

DOC_IMAGE := centrifugo-docs
DOCKER_RUN_DOC_PORT := 8000
DOCKER_RUN_DOC_MOUNT := -v $(CURDIR)/docs:/mkdocs
DOCKER_RUN_DOC_OPTS := --rm $(DOCKER_RUN_DOC_MOUNT) -p $(DOCKER_RUN_DOC_PORT):8000

all: test

prepare:
	go install github.com/mitchellh/gox@latest

test:
	go test -count=1 -v $(TESTFOLDERS) -cover -race

test-integration:
	go test -count=1 -v $(TESTFOLDERS) -cover -race --tags=integration

generate:
	go generate ./...
	bash misc/scripts/generate.sh

web:
	./misc/scripts/update_web.sh

swagger-web:
	./misc/scripts/update_swagger_web.sh

package:
	./misc/scripts/package.sh $(VERSION) $(ITERATION)

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/stretch PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/buster PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/bullseye PACKAGES/*.deb

	package_cloud push FZambia/centrifugo/ubuntu/xenial PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/bionic PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/focal PACKAGES/*.deb

packagecloud-rpm:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/el/7 PACKAGES/*.rpm
	package_cloud push FZambia/centrifugo/el/8 PACKAGES/*.rpm

docs: docs-image
	docker run $(DOCKER_RUN_DOC_OPTS) $(DOC_IMAGE) mkdocs serve

docs-deploy: docs-image
	cd docs && mkdocs gh-deploy

docs-image:
	docker build -t $(DOC_IMAGE) -f docs/Dockerfile docs/

docs-env-create:
	python3 -m venv .venv

deps:
	go mod tidy

local-deps:
	go mod tidy
	go mod download
	go mod vendor

build:
	CGO_ENABLED=0 go build
