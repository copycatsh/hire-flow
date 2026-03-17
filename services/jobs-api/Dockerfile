FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8001
CMD ["/server"]
