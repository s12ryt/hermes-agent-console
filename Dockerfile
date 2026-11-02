# 前端建置階段：產出 web/dist 供 go:embed 使用
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Go 建置階段：交叉編譯單一靜態執行檔
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed.go web/embed.go
COPY --from=web /src/web/dist web/dist/
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/hermes-console ./cmd/console

# 執行階段：包含 Python，供掛載進容器的 Hermes 環境使用
FROM python:3.12-slim
LABEL org.opencontainers.image.source=https://github.com/s12ryt/hermes-agent-console
LABEL org.opencontainers.image.description="Personal management console for the Hermes agent"
COPY --from=build /out/hermes-console /usr/local/bin/hermes-console
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/hermes-console"]
CMD ["-addr", "0.0.0.0:8080", "-data", "/data"]
