FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/outbox/go.mod pkg/outbox/go.sum ./pkg/outbox/
COPY services/payments/go.mod services/payments/go.sum ./services/payments/
RUN cd services/payments && go mod download
COPY pkg/outbox/ ./pkg/outbox/
COPY services/payments/ ./services/payments/
RUN cd services/payments && CGO_ENABLED=0 go build -o /server ./cmd/server
RUN cd services/payments && CGO_ENABLED=0 go build -o /seed ./cmd/seed

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
COPY --from=build /seed /seed
EXPOSE 8004
CMD ["/server"]
