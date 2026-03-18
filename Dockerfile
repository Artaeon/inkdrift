FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o inkdrift .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/inkdrift .
COPY templates/ ./templates/

RUN mkdir -p /data

ENV INKDRIFT_DB_PATH=/data/inkdrift.db

EXPOSE 3377

VOLUME ["/data"]

ENTRYPOINT ["./inkdrift"]
CMD ["serve"]
