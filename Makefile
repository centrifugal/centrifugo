VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')
ITERATION := $(shell git describe --tags --long | awk -F "-" '{print $$2}')
TESTFOLDERS := $(shell go list ./... | grep -v /vendor/ | grep -v /extras/)

DOC_IMAGE := centrifugo-docs
DOCKER_RUN_DOC_PORT := 8000
DOCKER_RUN_DOC_MOUNT := -v $(CURDIR)/docs:/mkdocs
DOCKER_RUN_DOC_OPTS := --rm $(DOCKER_RUN_DOC_MOUNT) -p $(DOCKER_RUN_DOC_PORT):8000

all: release

release:
	@read -p "Enter new release version: " version; \
	./extras/scripts/release.sh $$version

prepare:
	go get github.com/mitchellh/gox

test:
	go test $(TESTFOLDERS) -cover

web:
	./extras/scripts/update_web.sh

bindata:
	statik -src=extras/web -dest ./lib/ -package=webui

package:
	./extras/scripts/package.sh $(VERSION) $(ITERATION)

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/wheezy PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/jessie PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/stretch PACKAGES/*.deb

	package_cloud push FZambia/centrifugo/ubuntu/trusty PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/xenial PACKAGES/*.deb

packagecloud-rpm:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/el/7 PACKAGES/*.rpm
	package_cloud push FZambia/centrifugo/el/6 PACKAGES/*.rpm

docs: docs-image
	docker run  $(DOCKER_RUN_DOC_OPTS) $(DOC_IMAGE) mkdocs serve

docs-image:
	docker build -t $(DOC_IMAGE) -f docs/Dockerfile docs/
