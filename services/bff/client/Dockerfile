FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/bff/client/go.mod services/bff/client/go.sum ./services/bff/client/
RUN cd services/bff/client && go mod download
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/bff/client/ ./services/bff/client/
RUN cd services/bff/client && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8010
CMD ["/server"]
