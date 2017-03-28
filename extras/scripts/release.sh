#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

MAIN_DIR=`pwd`
DOCKERFILE=$MAIN_DIR/Dockerfile
DOCKERFILE_TEMPLATE=$MAIN_DIR/extras/scripts/dockerfile.template
HOMEBREWFILE=$MAIN_DIR/centrifugo.rb
HOMEBREWFILE_TEMPLATE=$MAIN_DIR/extras/scripts/homebrew.template

mkdir -p BUILDS
mkdir -p BUILDS/$1
rm -rf BUILDS/$1/*

gox -os="linux darwin freebsd windows" -arch="amd64 386 arm" -ldflags="-X main.VERSION=$1" -output="./BUILDS/$1/centrifugo-$1-{{.OS}}-{{.Arch}}/centrifugo"

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

CHECKSUM=`cat sha256sum.txt | grep "darwin-amd64" | awk -F "  " '{print $1}'`
echo "SHA 256 sum for Homebrew formula file: $CHECKSUM"
sed -e "s;%version%;$1;g" -e "s;%checksum%;$CHECKSUM;g" $HOMEBREWFILE_TEMPLATE > $HOMEBREWFILE
echo "Homebrew formula updated"

echo "Done! Now what you should do:"
echo "1) Write CHANGELOG.md if needed"
echo "2) Commit and push changes"
echo "3) Create and push tag v$1"
echo "4) Upload binaries from BUILDS/$1 to Github tag release"

