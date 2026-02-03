FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/worker ./cmd/worker

FROM alpine:3.20
COPY --from=build /bin/worker /bin/worker
ENTRYPOINT ["/bin/worker"]
