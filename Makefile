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

web:
	./misc/scripts/update_web.sh

swagger-web:
	make generate
	./misc/scripts/update_swagger_web.sh

package:
	./misc/scripts/package.sh $(VERSION) $(ITERATION)

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/buster PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/bullseye PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/bookworm PACKAGES/*.deb

	package_cloud push FZambia/centrifugo/ubuntu/bionic PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/focal PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/jammy PACKAGES/*.deb

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
