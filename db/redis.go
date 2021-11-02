package db

import (
	"bufio"
	"fmt"
	"net/http"
	"os"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/gomodule/redigo/redis"
)

var Cache redis.Conn

func InitCache() error {
	conn, err := redis.DialURL(config.Redis)
	Cache = conn
	return err
}

func CloseCache() error {
	return Cache.Close()
}

func CheckSession(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	c, err := r.Cookie("session_token")

	if err != nil {
		if err == http.ErrNoCookie {
			w.WriteHeader(http.StatusUnauthorized)
			return nil, err
		}

		w.WriteHeader(http.StatusBadRequest)
		return nil, err
	}

	sessionToken := c.Value

	response, err := Cache.Do("GET", sessionToken)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return nil, err
	}
	if response == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, err
	}

	return response, nil
}

func GetClientKey() (string, error) {
	file, err := os.Open("clientkey")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var line string
	for scanner.Scan() {
		line = fmt.Sprintf("%s", scanner.Text())
	}

	return line, nil
}
