# syntax=docker/dockerfile:1

# Stage 1: сборка SPA
FROM node:24-alpine3.23@sha256:0efee1e068d21f6edcab7fa8c962fc4ecd50d4e28727bc23963379b63ee318ab AS frontend-build
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: сборка backend (Go >= 1.25; CGO не используется — бинарник статический)
FROM golang:1.26-alpine3.23@sha256:78e7f1e419545bc4b8b5ee8a40a99ddb5df1216d38681dbab2eeb1fdb2427f43 AS backend-build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# Stage 3: runtime
FROM alpine:3.23@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2
RUN addgroup -S svoyshef && adduser -S -G svoyshef -u 10001 svoyshef
RUN mkdir -p /app/backend /app/frontend /app/data
COPY --from=backend-build /out/server /app/backend/server
COPY --from=frontend-build /src/dist /app/frontend/dist
# photos/ исключены через .dockerignore: локальные фото живут на машине пилота (бинд-маунт ./data)
COPY data/aidabaran /app/data/aidabaran
# NFR-05: runtime contains only the server, built SPA and tracked restaurant data.
WORKDIR /app/backend
ENV RESTAURANT=aidabaran \
    DATA_DIR=/app/data \
    FRONTEND_DIST=/app/frontend/dist \
    PORT=8080
USER svoyshef
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/backend/server"]
