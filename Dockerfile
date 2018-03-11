FROM alpine:3.7

RUN apk --no-cache upgrade && apk --no-cache add ca-certificates

COPY centrifugo /usr/local/bin/centrifugo 

CMD ["centrifugo"]

