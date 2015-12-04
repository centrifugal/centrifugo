FROM gliderlabs/alpine:3.2

ENV VERSION 1.1.0

ENV DOWNLOAD https://github.com/centrifugal/centrifugo/releases/download/v$VERSION/centrifugo-$VERSION-linux-amd64.zip

RUN apk --update add unzip \
    && addgroup -S centrifugo && adduser -S -G centrifugo centrifugo \
    && mkdir /centrifugo && chown centrifugo:centrifugo /centrifugo \
    && mkdir /var/log/centrifugo && chown centrifugo:centrifugo /var/log/centrifugo

ADD ${DOWNLOAD} /tmp/centrifugo.zip

RUN unzip -jo /tmp/centrifugo.zip -d /tmp/ \
    && mv /tmp/centrifugo /usr/bin/centrifugo \
    && rm -f /tmp/centrifugo.zip \
    && apk del unzip \
    && rm -rf /var/cache/apk/*

VOLUME ["/centrifugo", "/var/log/centrifugo"]

WORKDIR /centrifugo

USER centrifugo

CMD ["centrifugo"]

EXPOSE 8000
