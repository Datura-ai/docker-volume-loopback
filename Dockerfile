# build
FROM golang:1.26-alpine AS builder
RUN apk --no-cache add make
ADD ./ /go/src/github.com/ashald/docker-volume-loopback/
WORKDIR /go/src/github.com/ashald/docker-volume-loopback
RUN CGO_ENABLED=0 make build && \
    mv ./docker-volume-loopback /


# package
FROM alpine:3.24
RUN apk --no-cache add \
    # fs detection
    file \
    # ext4
    e2fsprogs \
    # xfs
    xfsprogs util-linux \
    # terminfo files are shipped with 'util-linux' and are hardlinks - that breaks docker export tar
    && rm -rf /usr/share/terminfo \
    && rm -rf /etc/terminfo

COPY --from=builder /docker-volume-loopback /
CMD [ "/docker-volume-loopback" ]
