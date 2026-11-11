# Postgres 17 with pgvector and pg_textsearch (BM25).
#
# pg_textsearch ships prebuilt Debian packages per Postgres major and per CPU, so
# this image installs the release instead of compiling it. Each download is pinned
# by SHA-256: a moved or replaced release asset fails the build instead of
# shipping silently. Bumping the version means updating the version and both sums
# (the release page lists them as the asset digests).
#
# The extension must be preloaded, hence the CMD: without
# shared_preload_libraries the bm25 access method is not available.
FROM alpine:3.20 AS fetch
ARG TARGETARCH
ARG PG_TEXTSEARCH_VERSION=1.5.1
ARG PG_TEXTSEARCH_SHA256_AMD64=ed4410e8b6879a8971cd87fd03853f182e47b85b199c9bfcd9b34f159caba9fb
ARG PG_TEXTSEARCH_SHA256_ARM64=e65f953e99f47c05dde88a031884cf36ea269bcb62377f554b76ddb4d13fdc3e
RUN apk add --no-cache curl unzip
RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64) sha="${PG_TEXTSEARCH_SHA256_AMD64}" ;; \
      arm64) sha="${PG_TEXTSEARCH_SHA256_ARM64}" ;; \
      *) echo "pg_textsearch has no prebuilt package for '${TARGETARCH}'" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/pgts.zip \
      "https://github.com/timescale/pg_textsearch/releases/download/v${PG_TEXTSEARCH_VERSION}/pg-textsearch-v${PG_TEXTSEARCH_VERSION}-pg17-${TARGETARCH}.zip"; \
    echo "${sha}  /tmp/pgts.zip" | sha256sum -c -; \
    mkdir /out; unzip -j /tmp/pgts.zip -d /out

# Trixie, not the default bookworm tag: the prebuilt package needs glibc >= 2.38
# and bookworm ships 2.36, so the server refuses to start with
# "GLIBC_2.38 not found".
FROM pgvector/pgvector:pg17-trixie
COPY --from=fetch /out/*.deb /tmp/
# dpkg, not apt: the image carries no package lists and the only dependency
# (postgresql-17) is already installed, so nothing needs the network.
RUN dpkg -i /tmp/pg-textsearch-*.deb && rm -f /tmp/*.deb
CMD ["postgres", "-c", "shared_preload_libraries=pg_textsearch"]
