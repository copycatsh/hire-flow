FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/bff/go.mod pkg/bff/go.sum ./pkg/bff/
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/bff/admin/go.mod services/bff/admin/go.sum ./services/bff/admin/
RUN cd services/bff/admin && go mod download
COPY pkg/bff/ ./pkg/bff/
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/bff/admin/ ./services/bff/admin/
RUN cd services/bff/admin && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8012
CMD ["/server"]
