# Copyright Josh Komoroske. All rights reserved.
# Use of this source code is governed by the MIT license,
# a copy of which can be found in the LICENSE.txt file.
# SPDX-License-Identifier: MIT

# The certs stage is used to obtain a current set of CA certificates.
FROM alpine:3.16 as certs

# hadolint ignore=DL3018
RUN apk add --no-cache \
    ca-certificates

# The builder build stage compiles the Go code into a static binary.
FROM golang:1.19-alpine as builder

ARG VERSION=development

WORKDIR /go/src/github.com/joshdk/drone-github-comment

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o /bin/drone-github-comment \
    -ldflags "-s -w -X main.version=$VERSION" \
    -trimpath \
    main.go

# The upx build stage uses upx to compress the binary.
FROM alpine:3.16 as upx

RUN wget https://github.com/upx/upx/releases/download/v3.96/upx-3.96-amd64_linux.tar.xz \
 && tar -xf upx-3.96-amd64_linux.tar.xz \
 && install upx-3.96-amd64_linux/upx /bin/upx \
 && rm -rf upx*

COPY --from=builder /bin/drone-github-comment /bin/drone-github-comment

RUN upx --best --ultra-brute /bin/drone-github-comment

# The final build stage copies in the final binary.
FROM scratch

ARG CREATED
ARG REVISION
ARG VERSION

MAINTAINER Josh Komoroske <github.com/joshdk>

# Standard OCI image labels.
# See: https://github.com/opencontainers/image-spec/blob/v1.0.1/annotations.md#pre-defined-annotation-keys
LABEL org.opencontainers.image.created="$CREATED"
LABEL org.opencontainers.image.authors="Josh Komoroske <github.com/joshdk>"
LABEL org.opencontainers.image.url="https://github.com/joshdk/drone-github-comment"
LABEL org.opencontainers.image.documentation="https://github.com/joshdk/drone-github-comment/blob/master/README.md"
LABEL org.opencontainers.image.source="https://github.com/joshdk/drone-github-comment"
LABEL org.opencontainers.image.version="$VERSION"
LABEL org.opencontainers.image.revision="$REVISION"
LABEL org.opencontainers.image.vendor="Josh Komoroske <github.com/joshdk>"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.ref.name="ghcr.io/joshdk/drone-github-comment:$VERSION"
LABEL org.opencontainers.image.title="Drone Github Comment Plugin"
LABEL org.opencontainers.image.description="Drone plugin which takes the output of a step and comments on a Github pull request"

COPY LICENSE.txt /
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY README.md /
COPY --from=upx /bin/drone-github-comment /bin/drone-github-comment

# Switch to a non-root user. Arbitrarily, use the same uid/gid as the "nobody"
# user from Alpine.
USER 65534:65534

ENTRYPOINT ["/bin/drone-github-comment"]
