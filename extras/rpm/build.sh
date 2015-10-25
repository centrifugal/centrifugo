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

cp centrifugo-$1-linux-amd64.zip ~/rpmbuild/SOURCES/centrifugo-$1-linux-amd64.zip
cp centrifugo.spec ~/rpmbuild/SPECS/centrifugo.spec
cp centrifugo.initd ~/rpmbuild/SOURCES/centrifugo.initd
cp centrifugo.nofiles.conf ~/rpmbuild/SOURCES/centrifugo.nofiles.conf
cp centrifugo.logrotate ~/rpmbuild/SOURCES/centrifugo.logrotate
cp centrifugo.config.json ~/rpmbuild/SOURCES/centrifugo.config.json

rpmbuild -bb ~/rpmbuild/SPECS/centrifugo.spec --define "version $1" --define "release `date +%s`"
