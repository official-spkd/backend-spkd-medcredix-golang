# Backend MedCredix (Go). Build 2 tahap: compile, lalu image kecil tanpa compiler.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# timetzdata: zona waktu ikut di binary (Asia/Jakarta)
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w" -o /out/medcredix ./cmd/server

FROM alpine:3.22
RUN adduser -D -H -u 10001 app && mkdir -p /app/uploads && chown app /app/uploads
WORKDIR /app
COPY --from=build /out/medcredix /app/medcredix
USER app
ENV MEDCREDIX_PORT=8080 UPLOAD_DIR=/app/uploads
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1
CMD ["/app/medcredix"]
