package main

import (
	"bufio"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"net/http"
	"os"
	"strings"
)

var cache redis.Conn

func InitCache() {
	conn, err := redis.DialURL(GetConfigValue("redis", "redis://localhost"))
	if err != nil {
		panic(err)
	}
	cache = conn
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

	response, err := cache.Do("GET", sessionToken)

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

func GetClientKey() string {
	file, err := os.Open("clientkey")

	CheckError(err, "could not open client key in file")

	defer file.Close()

	scanner := bufio.NewScanner(file)
	var line string
	for scanner.Scan() {
		line = fmt.Sprintf("%s", scanner.Text())
	}

	return line
}

func GetPasswordFromSession(r *http.Request) (string, string) {

	c, err := r.Cookie("session_token")

	if err != nil {
		return "", ""
	}

	sessionToken := c.Value

	response, err := cache.Do("GET", sessionToken)

	if CheckError(err, "could not get session from cache") != nil {
		return "", ""
	}

	token := fmt.Sprintf("%s", response)

	parts := strings.Split(token, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}
