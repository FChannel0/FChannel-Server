FROM golang:1.16-alpine AS builder
WORKDIR /build
COPY . .
RUN go build .

FROM alpine:3.14
RUN apk --no-cache add imagemagick exiv2 ttf-opensans
WORKDIR /app
COPY --from=builder /build/Server /app
COPY static/ /app/static/
COPY databaseschema.psql /app
CMD ["/app/Server"]
