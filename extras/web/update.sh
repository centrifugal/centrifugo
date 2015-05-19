#!/bin/bash
if [ -d ./app ]; then
	rm -rf ./app
fi
mkdir -p ./app
git clone https://github.com/centrifugal/web.git /tmp/centrifugo-web/
cp -R /tmp/centrifugo-web/app/ ./app
rm -rf ./app/src
rm -rf /tmp/centrifugo-web/
