# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.25-alpine AS build
WORKDIR /src

# cache deps
COPY go.mod go.sum ./
RUN go mod download

# build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/feeds .

# --- runtime stage ---
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/feeds /app/feeds

ENV HOST=0.0.0.0 PORT=3000
EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/app/feeds"]
