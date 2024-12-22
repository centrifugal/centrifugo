FROM alpine:3.21

ARG USER=centrifugo
ARG UID=1000
ARG GID=1000

RUN addgroup -S -g $GID $USER && \
    adduser -S -G $USER -u $UID $USER

RUN apk --no-cache upgrade && \
    apk --no-cache add ca-certificates && \
    update-ca-certificates

USER $USER

WORKDIR /centrifugo

COPY centrifugo /usr/local/bin/centrifugo

CMD ["centrifugo"]
