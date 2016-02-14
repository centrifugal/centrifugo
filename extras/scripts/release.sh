#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

MAIN_DIR=`pwd`
VERSION_FILE=$MAIN_DIR/version.go
DOCKERFILE=$MAIN_DIR/Dockerfile
DOCKERFILE_TEMPLATE=$MAIN_DIR/extras/scripts/dockerfile.template

cat > $VERSION_FILE <<EOF
package main

const (
	// VERSION of Centrifugo server.
	VERSION = "$1"
)
EOF

mkdir -p BUILDS
mkdir -p BUILDS/$1
rm -rf BUILDS/$1/*

gox -os="linux" -output="./BUILDS/$1/centrifugo-$1-{{.OS}}-{{.Arch}}/centrifugo"

cd BUILDS/$1

touch sha256sum.txt

for i in */; do
  zip -r "${i%/}.zip" "$i"
  shasum -a 256 "${i%/}.zip" >> sha256sum.txt
  rm -r $i
done

CHECKSUM=`cat sha256sum.txt | grep "linux-amd64" | awk -F "  " '{print $1}'`
echo "SHA 256 sum for Dockerfile: $CHECKSUM"

sed -e "s;%version%;$1;g" -e "s;%checksum%;$CHECKSUM;g" $DOCKERFILE_TEMPLATE > $DOCKERFILE
echo "Centos 7 Dockerfile updated"

echo "Done!"

