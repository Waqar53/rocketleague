FROM golang:1.25.6-alpine AS builder
WORKDIR /src
COPY . .
ARG SERVICE
RUN test -n "$SERVICE"
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/service /service
EXPOSE 9000 9001 9002 9003
ENTRYPOINT ["/service"]
