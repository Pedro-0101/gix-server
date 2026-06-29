# Build do gix-server. Fase 1: binário Go 100% estático (pgx é Go puro, sem CGO,
# sem ONNX ainda). Quando a fase 2 trouxer embeddings (onnxruntime via purego +
# libonnxruntime.so), a imagem final precisará de glibc — trocar o estágio final
# por debian-slim e copiar a lib.

FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
