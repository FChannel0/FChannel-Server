VERSION=`git describe --tags`
BUILD=`date +%FT%T%z`

LDFLAGS=-X github.com/FChannel0/FChannel-Server/config.Version=${VERSION} -X github.com/FChannel0/FChannel-Server/config.BuildTime=${BUILD}
FLAGS=-ldflags "-w -s ${LDFLAGS}"
FLAGS_DEBUG=-ldflags "${LDFLAGS}"

debug:
	go build -o fchan ${FLAGS_DEBUG}

build:
	go build -o fchan ${FLAGS}

clean:
	if [ -f "fchan" ]; then rm "fchan"; fi

.PHONY: clean install
