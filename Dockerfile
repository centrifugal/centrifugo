FROM centos:7

ENV VERSION 1.7.5

ENV CENTRIFUGO_SHA256 63ea1c92801117f0d143fc92e4dfd34f58d7c22c97d1508ab0ca986acf33732b

ENV DOWNLOAD https://github.com/centrifugal/centrifugo/releases/download/v$VERSION/centrifugo-$VERSION-linux-amd64.zip

RUN curl -sSL "$DOWNLOAD" -o /tmp/centrifugo.zip && \
    echo "${CENTRIFUGO_SHA256}  /tmp/centrifugo.zip" | sha256sum -c - && \
    yum install -y unzip && \
    unzip -jo /tmp/centrifugo.zip -d /tmp/ && \
    yum remove -y unzip && \
    mv /tmp/centrifugo /usr/bin/centrifugo && \
    rm -f /tmp/centrifugo.zip && \
    echo "centrifugo - nofile 65536" >> /etc/security/limits.d/centrifugo.nofiles.conf

RUN groupadd -r centrifugo && useradd -r -g centrifugo centrifugo

RUN mkdir /centrifugo && chown centrifugo:centrifugo /centrifugo && \
    mkdir /var/log/centrifugo && chown centrifugo:centrifugo /var/log/centrifugo

VOLUME ["/centrifugo", "/var/log/centrifugo"]

WORKDIR /centrifugo

USER centrifugo

CMD ["centrifugo"]

EXPOSE 8000

