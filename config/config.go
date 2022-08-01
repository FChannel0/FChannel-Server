package config

import (
	"bufio"
	"database/sql"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"strconv"
	"strings"
)

type InstanceConfig struct {
	Domain      string `yaml:"domain"`
	Port        int    `yaml:"port"`
	Protocol    string `yaml:"protocol"`
	Name        string `yaml:"name"`
	Summary     string `yaml:"summary"`
	Salt        string `yaml:"salt"`
	MaxFilesize int64  `yaml:"max_filesize"`
}
type EmailConfig struct {
	Address  string   `yaml:"address"`
	Password string   `yaml:"password"`
	Host     string   `yaml:"post"`
	Port     int      `yaml:"Port"`
	Notify   []string `yaml:"Notify"`
}
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type AllConfig struct {
	Instance       InstanceConfig `yaml:"instance"`
	Email          EmailConfig    `yaml:"email"`
	Database       DatabaseConfig `yaml:"database"`
	TorProxy       string         `yaml:"tor_proxy"`
	CookieKey      string         `yaml:"cookie_key,omitempty"`
	AuthReq        []string       `yaml:"auth_req"`
	PostPerPage    int            `yaml:"posts_per_page"`
	SupportedFiles []string       `yaml:"supported_files"`
	ModKey         string         `yaml:"mod_key"`
}

var yamlData = AllConfig{
	Instance: InstanceConfig{
		Domain:      "",
		Port:        3000,
		Protocol:    "https",
		Name:        "",
		Summary:     "",
		Salt:        "",
		MaxFilesize: 8,
	},
	Email: EmailConfig{
		Address:  "",
		Password: "",
		Host:     "localhost",
		Port:     587,
		Notify:   []string{},
	},
	Database: DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "postgres",
		User:     "postgres",
		Password: "",
	},
	TorProxy:    "127.0.0.1:9050",
	CookieKey:   "",
	AuthReq:     []string{"captcha", "Email", "passphrase"},
	PostPerPage: 10,
	SupportedFiles: []string{
		"image/gif", "image/jpeg", "image/png", "image/webp", "image/apng",
		"video/mp4", "video/ogg", "video/webm",
		"audio/mpeg", "audio/ogg", "audio/wav", "audio/wave", "audio/x-wav"},
	ModKey: "",
}

var Port string
var TP string
var Domain string
var InstanceName string
var InstanceSummary string
var Salt string
var MaxFilesize int64

var SiteEmail string //contact@fchan.xyz
var SiteEmailPassword string
var SiteEmailServer string //mail.fchan.xyz
var SiteEmailPort string   //587
var SiteEmailNotifyTo string

var DBHost string
var DBPort int
var DBUser string
var DBPassword string
var DBName string

var TorProxy string //127.0.0.1:9050
var CookieKey string
var AuthReq []string
var PostCountPerPage int
var SupportedFiles []string
var Key string

var ActivityStreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""
var Log = log.New(os.Stdout, "", log.Ltime)
var MediaHashs = make(map[string]string)
var Themes []string
var DB *sql.DB

func LoadConfig() {

	file, err := os.ReadFile("config/config-init.yaml")

	if os.IsNotExist(err) {
		Log.Println("Failed to read config/config-init.yaml, creating new")
		if _, err := os.Stat("config/config-init"); err == nil {
			migrateOldConfig()
		}

		data, err2 := yaml.Marshal(&yamlData)
		if err2 != nil {
			Log.Fatalln(err2)
		}

		err2 = os.WriteFile("config/config-init.yaml", data, 0640)
		if err2 != nil {
			Log.Fatalln(err2)
		}

		Log.Println("New config written to config/config-init.yaml")
	} else if err != nil {
		Log.Fatalln(err)
	}

	err = yaml.Unmarshal(file, &yamlData)
	if err != nil {
		Log.Fatalln(err)
	}

	// Instance configuration
	TP = yamlData.Instance.Protocol + "://"
	Domain = TP + yamlData.Instance.Domain
	Port = ":" + strconv.Itoa(yamlData.Instance.Port)
	InstanceName = yamlData.Instance.Name
	InstanceSummary = yamlData.Instance.Summary
	MaxFilesize = yamlData.Instance.MaxFilesize

	// Email configuration
	SiteEmail = yamlData.Email.Address
	SiteEmailPassword = yamlData.Email.Password
	SiteEmailServer = yamlData.Email.Host
	SiteEmailPort = strconv.Itoa(yamlData.Email.Port)
	SiteEmailNotifyTo = strings.Join(yamlData.Email.Notify, ",")

	// Database configuration
	DBHost = yamlData.Database.Host
	DBPort = yamlData.Database.Port
	DBName = yamlData.Database.Name
	DBUser = yamlData.Database.User
	DBPassword = yamlData.Database.Password

	// Other configuration
	TorProxy = yamlData.TorProxy
	CookieKey = yamlData.CookieKey
	AuthReq = yamlData.AuthReq
	PostCountPerPage = yamlData.PostPerPage
	SupportedFiles = yamlData.SupportedFiles
	Key = yamlData.ModKey
}

func migrateOldConfig() {
	Log.Println("config/config-init found, migrating old config")

	yamlData.Instance.Port, _ = strconv.Atoi(getConfigValue("instanceport", "3000"))
	yamlData.Instance.Protocol = strings.TrimSuffix(getConfigValue("instancetp", "https"), "://")
	yamlData.Instance.Name = getConfigValue("instancename", "")
	yamlData.Instance.Summary = getConfigValue("instancesummary", "")
	yamlData.Instance.Salt = getConfigValue("instancesalt", "")

	yamlData.Email.Address = getConfigValue("emailaddress", "")
	yamlData.Email.Password = getConfigValue("emailpass", "")
	yamlData.Email.Host = getConfigValue("emailserver", "")
	yamlData.Email.Port, _ = strconv.Atoi(getConfigValue("emailport", "587"))
	yamlData.Email.Notify = strings.Split(getConfigValue("emailnotify", ""), ",")

	yamlData.Database.Host = getConfigValue("dbhost", "localhost")
	yamlData.Database.Port, _ = strconv.Atoi(getConfigValue("dbport", "5432"))
	yamlData.Database.Name = getConfigValue("dbname", "postgres")
	yamlData.Database.User = getConfigValue("dbuser", "postgres")
	yamlData.Database.Password = getConfigValue("dbpass", "")

	yamlData.TorProxy = getConfigValue("TorProxy", "127.0.0.1:9050")
	yamlData.CookieKey = getConfigValue("CookieKey", "")
	yamlData.ModKey = getConfigValue("ModKey", "")
}

func getConfigValue(value string, ifnone string) string {
	file, err := os.Open("config/config-init")

	if err != nil {
		Log.Fatalln(err)
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
