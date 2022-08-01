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

ENV INSTANCE_DOMAIN="localhost"
ENV INSTANCE_PORT=3000
ENV INSTANCE_PROTOCOL="https"
ENV INSTANCE_NAME=""
ENV INSTANCE_SUMMARY=""
ENV INSTANCE_SALT=""
ENV INSTANCE_FILESIZE_LIMIT=8

ENV EMAIL_ADDRESS=""
ENV EMAIL_PASSWORD=""
ENV EMAIL_HOST="localhost"
ENV EMAIL_PORT=587
ENV EMAIL_NOTIFY=""

ENV DB_HOST="localhost"
ENV DB_PORT=5432
ENV DB_NAME="postgres"
ENV DB_USER="postgres"
ENV DB_PASS=""

ENV TOR_PROXY="127.0.0.1:9050"
ENV COOKIE_KEY=""
ENV AUTH_REQ="captcha,Email,passphrase"
ENV POSTS_PER_PAGE=10
ENV SUPPORTED_FILES="image/gif,image/jpeg,image/png,image/webp,image/png,video/mp4,video/ogg,video/webm,audio/mpeg,audio/ogg,audio/wav,audio/wave,audio/x-wav"
ENV MOD_KEY=""

EXPOSE ${INSTANCE_PORT}

CMD ["/app/fchan"]
