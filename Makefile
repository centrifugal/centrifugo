VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')
ITERATION := $(shell git describe --tags | awk -F "-" '{print $$2}')	

all: release

release:
	@read -p "Enter new release version: " version; \
	./extras/scripts/release.sh $$version

prepare:
	go get github.com/mitchellh/gox
	go get github.com/tools/godep
	godep restore

test:
	go test ./... -cover

bindata:
	go-bindata-assetfs -prefix="extras/web" extras/web/app/...
	mv bindata_assetfs.go bindata.go
	gofmt -w bindata.go	
	godep save -r ./...

packages:
	./extras/scripts/package.sh $(VERSION) $(ITERATION)

packagecloud:
	make packagecloud-deb
	make packagecloud-rpm

packagecloud-deb:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/debian/wheezy *.deb
	package_cloud push FZambia/centrifugo/debian/jessie *.deb

	package_cloud push FZambia/centrifugo/ubuntu/precise *.deb
	package_cloud push FZambia/centrifugo/ubuntu/trusty *.deb
	package_cloud push FZambia/centrifugo/ubuntu/xenial *.deb

packagecloud-rpm:
	# PACKAGECLOUD_TOKEN env must be set
	package_cloud push FZambia/centrifugo/el/7 *.rpm
	package_cloud push FZambia/centrifugo/el/6 *.rpm
