# Base images are pinned by digest to remove floating-tag drift as a source
# of CI network flakes (a tag re-pull can hit a transient registry error at
# an unpredictable time; a digest pin makes the pull content-addressed and
# retry-safe). Digests resolved 2026-09-19 via:
#   docker buildx imagetools inspect <image>:<tag>
# Refresh procedure: re-run that command, replace the digest below, and keep
# the tag suffix for human readability.
FROM node:22-bookworm@sha256:dd5847a04b0deee391fa145f1f4c6d214196668b6bcc7988ebed67249f226844 AS node-runtime

FROM golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    git \
    gnupg \
    pkg-config \
    time \
    && rm -rf /var/lib/apt/lists/*

# Debian's git links against GnuTLS. GnuTLS resolves its default "system
# trust" through the p11-kit PKCS#11 trust module instead of reading the CA
# bundle file directly. Under this harness's process churn (many short-lived
# git clones per exhaustive parity run), that indirection has produced
# intermittent "server certificate verification failed. CAfile: none" clone
# failures against C-oracle grammar repos that need a fresh network clone.
# Pointing git straight at the installed CA bundle skips the p11-kit lookup
# and gives every git subprocess in the container a fixed, known-good file to
# verify against.
ENV GIT_SSL_CAINFO=/etc/ssl/certs/ca-certificates.crt

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# tree-sitter-cli stays on 0.24.7. Neither cgo_harness/README.md nor BENCH.md
# binds this version to the 0.25 runtime oracle (go-tree-sitter v0.25.0 /
# upstream runtime v0.25.1 documented in BENCH.md cover the parity runtime
# only, not the grammar-generation CLI). parity_c_loader_cgo.go already
# targets ABI 15 first and falls back through lower ABI values when the
# installed CLI cannot emit it, so 0.24.7 works inside that negotiated
# range today. This change is scoped to CI network flakes; a CLI version
# bump needs its own parity validation pass and stays out of this change.
RUN npm --global --loglevel=warn install tree-sitter-cli@0.24.7

WORKDIR /workspace

ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=1
ENV GO111MODULE=on
