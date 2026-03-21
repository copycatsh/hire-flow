FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY pkg/bff/go.mod pkg/bff/go.sum ./pkg/bff/
COPY services/bff/freelancer/go.mod services/bff/freelancer/go.sum ./services/bff/freelancer/
RUN cd services/bff/freelancer && go mod download
COPY pkg/telemetry/ ./pkg/telemetry/
COPY pkg/bff/ ./pkg/bff/
COPY services/bff/freelancer/ ./services/bff/freelancer/
RUN cd services/bff/freelancer && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8011
CMD ["/server"]
