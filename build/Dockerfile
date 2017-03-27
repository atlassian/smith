FROM alpine:3.5

RUN apk --no-cache add \
    ca-certificates

ADD bin/linux/smith /bin/smith

ENTRYPOINT ["smith"]
