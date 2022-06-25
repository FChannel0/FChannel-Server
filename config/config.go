package config

import (
	"bufio"
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
)

var Port = ":" + GetConfigValue("instanceport", "3000")
var TP = GetConfigValue("instancetp", "")
var Domain = TP + "" + GetConfigValue("instance", "")
var InstanceName = GetConfigValue("instancename", "")
var InstanceSummary = GetConfigValue("instancesummary", "")
var SiteEmail = GetConfigValue("emailaddress", "") //contact@fchan.xyz
var SiteEmailPassword = GetConfigValue("emailpass", "")
var SiteEmailServer = GetConfigValue("emailserver", "") //mail.fchan.xyz
var SiteEmailPort = GetConfigValue("emailport", "")     //587
var SiteEmailNotifyTo = GetConfigValue("emailnotify", "")
var TorProxy = GetConfigValue("torproxy", "") //127.0.0.1:9050
var Salt = GetConfigValue("instancesalt", "")
var DBHost = GetConfigValue("dbhost", "localhost")
var DBPort, _ = strconv.Atoi(GetConfigValue("dbport", "5432"))
var DBUser = GetConfigValue("dbuser", "postgres")
var DBPassword = GetConfigValue("dbpass", "password")
var DBName = GetConfigValue("dbname", "server")
var CookieKey = GetConfigValue("cookiekey", "")
var ActivityStreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""
var AuthReq = []string{"captcha", "email", "passphrase"}
var PostCountPerPage = 10
var SupportedFiles = []string{"image/gif", "image/jpeg", "image/png", "image/webp", "image/apng", "video/mp4", "video/ogg", "video/webm", "audio/mpeg", "audio/ogg", "audio/wav", "audio/wave", "audio/x-wav"}
var Log = log.New(os.Stdout, "", log.Ltime)
var MediaHashs = make(map[string]string)
var Key = GetConfigValue("modkey", "")
var Themes []string
var DB *sql.DB

// TODO Change this to some other config format like YAML
// to save into a struct and only read once
func GetConfigValue(value string, ifnone string) string {
	file, err := os.Open("config/config-init")

	if err != nil {
		Log.Println(err)
		return ifnone
	}

	defer file.Close()

	lines := bufio.NewScanner(file)

	for lines.Scan() {
		line := strings.SplitN(lines.Text(), ":", 2)
		if line[0] == value {
			return line[1]
		}
	}

	return ifnone
}
