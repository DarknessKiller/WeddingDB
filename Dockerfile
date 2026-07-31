# Stage 1: Build frontend
FROM node:22-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.26-alpine AS backend-build
ARG VERSION=dev
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o server ./cmd/server/

# Stage 3: Runtime
FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend-build /app/backend/server ./server
COPY --from=frontend-build /app/frontend/build ./static
RUN mkdir -p uploads
EXPOSE 8080
ENV PORT=8080
ENV STATIC_DIR=./static
CMD ["./server"]
