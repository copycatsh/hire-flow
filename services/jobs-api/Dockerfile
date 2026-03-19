FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/outbox/go.mod pkg/outbox/go.sum ./pkg/outbox/
COPY services/jobs-api/go.mod services/jobs-api/go.sum ./services/jobs-api/
RUN cd services/jobs-api && go mod download
COPY pkg/outbox/ ./pkg/outbox/
COPY services/jobs-api/ ./services/jobs-api/
RUN cd services/jobs-api && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8001
CMD ["/server"]
