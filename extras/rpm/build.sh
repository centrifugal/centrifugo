#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit 1
fi

if [ ! -f centrifugo-$1-linux-amd64.zip ]; then
	echo "No Centrifugo release found in current directory"
	exit 1
fi

rm -rf ./centrifuge-web
git clone https://github.com/centrifugal/centrifuge-web.git ./centrifuge-web
rm -rf ./centrifuge-web/app/src/

cp centrifugo-$1-linux-amd64.zip ~/rpmbuild/SOURCES/centrifugo-$1-linux-amd64.zip
cp centrifugo.spec ~/rpmbuild/SPECS/centrifugo.spec
cp centrifugo.initd ~/rpmbuild/SOURCES/centrifugo.initd
cp centrifugo.nofiles.conf ~/rpmbuild/SOURCES/centrifugo.nofiles.conf
cp centrifugo.logrotate ~/rpmbuild/SOURCES/centrifugo.logrotate
cp centrifugo.config.json ~/rpmbuild/SOURCES/centrifugo.config.json
cp -rf ./centrifuge-web/app ~/rpmbuild/SOURCES/web
rm -rf ./centrifuge-web

rpmbuild -bb ~/rpmbuild/SPECS/centrifugo.spec --define "version $1" --define "release `date +%s`"
