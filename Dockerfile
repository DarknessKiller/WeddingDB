# Stage 1: Build frontend
FROM node:24-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.27-alpine AS backend-build
ARG VERSION=dev
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o server ./cmd/server/
RUN mkdir -p /uploads


# Stage 3: Runtime
FROM gcr.io/distroless/static-debian13
WORKDIR /app
COPY --from=backend-build /app/backend/server ./server
COPY --from=backend-build /uploads ./uploads
COPY --from=frontend-build /app/frontend/build ./static
EXPOSE 8080
ENV PORT=8080
ENV STATIC_DIR=./static
CMD ["./server"]
