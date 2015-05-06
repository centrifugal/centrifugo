#!/bin/bash
if [ "$1" = "" ]
then
  echo "Usage: $0 <version>"
  exit
fi

mkdir -p BUILDS
mkdir -p BUILDS/$1
rm -rf BUILDS/$1/*

gox -os="linux darwin freebsd windows" -output="./BUILDS/$1/centrifugo-$1-{{.OS}}-{{.Arch}}/centrifugo"

cd BUILDS/$1

for i in */; do
  zip -r "${i%/}.zip" "$i"
  rm -r $i
done
