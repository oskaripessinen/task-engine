FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api ./cmd/api

FROM alpine:3.20
COPY --from=build /bin/api /bin/api
ENTRYPOINT ["/bin/api"]
