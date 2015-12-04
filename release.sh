#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

# Generate bindata.go containing embedded web interface files.
# go-bindata-assetfs -prefix="extras/web" extras/web/app/...
# mv bindata_assetfs.go bindata.go

mkdir -p BUILDS
mkdir -p BUILDS/$1
rm -rf BUILDS/$1/*

gox -os="linux darwin freebsd windows" -output="./BUILDS/$1/centrifugo-$1-{{.OS}}-{{.Arch}}/centrifugo"

cd BUILDS/$1

touch sha256sum.txt

for i in */; do
  zip -r "${i%/}.zip" "$i"
  shasum -a 256 "${i%/}.zip" >> sha256sum.txt
  rm -r $i
done

echo "SHA 256 sum for Dockerfile:"
cat sha256sum.txt | grep "linux-amd64"
