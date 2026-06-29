# Build do gix-server com suporte a embeddings ONNX (CGO_ENABLED=1).
# A imagem final usa debian:bookworm-slim por causa da glibc exigida pela
# libonnxruntime.

# ----------------------------------------
# Estágio 1: baixar libonnxruntime
# ----------------------------------------
FROM debian:bookworm-slim AS onnx-dl
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://github.com/microsoft/onnxruntime/releases/download/v1.21.0/onnxruntime-linux-x64-1.21.0.tgz \
    -o /tmp/onnx.tgz && \
    tar -xzf /tmp/onnx.tgz -C /tmp && \
    cp /tmp/onnxruntime-linux-x64-1.21.0/lib/libonnxruntime.so* /usr/lib/

# ----------------------------------------
# Estágio 2: compilar o binário
# ----------------------------------------
FROM golang:1.25 AS build
COPY --from=onnx-dl /usr/lib/libonnxruntime* /usr/lib/
RUN apt-get update && apt-get install -y gcc && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -o /out/server ./cmd/server

# ----------------------------------------
# Estágio 3: imagem final
# ----------------------------------------
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=onnx-dl /usr/lib/libonnxruntime* /usr/lib/
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
