FROM alpine:3.4

RUN apk --no-cache add \
    ca-certificates

ADD bin/linux/smith /bin/smith

ENTRYPOINT ["smith"]
