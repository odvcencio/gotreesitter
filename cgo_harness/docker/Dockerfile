FROM node:22-bookworm AS node-runtime

FROM golang:1.25-bookworm

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

RUN npm --global --loglevel=warn install tree-sitter-cli@0.24.7

WORKDIR /workspace

ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=1
ENV GO111MODULE=on
