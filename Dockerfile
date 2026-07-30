# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend

COPY frontend/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY frontend/ ./
RUN npm run build

FROM golang:1.26.5-alpine AS backend-builder
WORKDIR /src

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/peopleops ./cmd/main.go && \
    go build -trimpath -ldflags="-s -w" -o /out/dingtalk_stream ./cmd/dingtalk_stream

FROM python:3.12-slim AS runtime
WORKDIR /app

ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_DATE

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY tools/attendance_toolbox/requirements-runtime.txt /tmp/attendance-toolbox-requirements.txt
RUN --mount=type=cache,target=/root/.cache/pip \
    python -m venv /opt/attendance-toolbox-venv \
    && /opt/attendance-toolbox-venv/bin/pip install --no-cache-dir -r /tmp/attendance-toolbox-requirements.txt \
    && rm -f /tmp/attendance-toolbox-requirements.txt

ENV APP_ENV=production \
    GIN_MODE=release \
    PORT=8080 \
    TZ=Asia/Shanghai \
    ATTENDANCE_TOOLBOX_PYTHON=/opt/attendance-toolbox-venv/bin/python \
    ATTENDANCE_TOOLBOX_DIR=/app/tools/attendance_toolbox/python

COPY --from=backend-builder /out/peopleops /app/peopleops
COPY --from=backend-builder /out/dingtalk_stream /app/dingtalk_stream
COPY --from=frontend-builder /src/frontend/dist /app/frontend/dist
COPY tools/attendance_toolbox /app/tools/attendance_toolbox

RUN mkdir -p /app/uploads

EXPOSE 8080
VOLUME ["/app/uploads"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/health" || exit 1

CMD ["/app/peopleops"]
