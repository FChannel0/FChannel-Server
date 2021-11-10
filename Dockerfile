FROM golang:1.16-alpine AS builder
WORKDIR /build
COPY . .
RUN apk --no-cache add make git

# Use the 'build' make target when fiber branch is stable
RUN make debug

FROM alpine:3.14
RUN apk --no-cache add imagemagick exiv2 ttf-opensans
WORKDIR /app
COPY --from=builder /build/fchan /app
COPY static/ /app/static/
COPY views/ /app/views/
COPY databaseschema.psql /app
CMD ["/app/fchan"]
