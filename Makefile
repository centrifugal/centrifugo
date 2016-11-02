VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')
ITERATION := $(shell git describe --tags --long | awk -F "-" '{print $$2}')
TESTFOLDERS := $(shell go list ./... | grep -v /vendor/ | grep -v /extras/)

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
	statik -src=extras/web -dest ./libcentrifugo/
	gofmt -w libcentrifugo/statik/statik.go	

package:
	./extras/scripts/package.sh $(VERSION) $(ITERATION)

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/wheezy PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/debian/jessie PACKAGES/*.deb

	package_cloud push FZambia/centrifugo/ubuntu/trusty PACKAGES/*.deb
	package_cloud push FZambia/centrifugo/ubuntu/xenial PACKAGES/*.deb

packagecloud-rpm:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/el/7 PACKAGES/*.rpm
	package_cloud push FZambia/centrifugo/el/6 PACKAGES/*.rpm
