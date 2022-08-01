package config

import (
	"bufio"
	"database/sql"
	"github.com/caarlos0/env/v6"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"strconv"
	"strings"
)

type InstanceConfig struct {
	Domain      string `yaml:"domain"       env:"DOMAIN"         envDefault:"fchan.xyz"`
	Port        int    `yaml:"port"         env:"PORT"           envDefault:"3000"`
	Protocol    string `yaml:"protocol"     env:"PROTOCOL"       envDefault:"https"`
	Name        string `yaml:"name"         env:"NAME"           envDefault:"FChan"`
	Summary     string `yaml:"summary"      env:"SUMMARY"        envDefault:"FChan is a federated image board instance."`
	Salt        string `yaml:"salt"         env:"SALT"`
	MaxFilesize int64  `yaml:"max_filesize" env:"FILESIZE_LIMIT" envDefault:"8"`
}
type EmailConfig struct {
	Address  string   `yaml:"address"  env:"ADDRESS"`
	Password string   `yaml:"password" env:"PASSWORD"`
	Host     string   `yaml:"post"     env:"HOST"`
	Port     int      `yaml:"Port"     env:"PORT"`
	Notify   []string `yaml:"Notify"   env:"NOTIFY"    envSeparator:","`
}
type DatabaseConfig struct {
	Host     string `yaml:"host"     env:"HOST" envDefault:"localhost"`
	Port     int    `yaml:"port"     env:"PORT" envDefault:"5432"`
	Name     string `yaml:"name"     env:"NAME" envDefault:"postgres"`
	User     string `yaml:"user"     env:"USER" envDefault:"postgres"`
	Password string `yaml:"password" env:"PASS"`
}

type AllConfig struct {
	Instance       InstanceConfig `yaml:"instance"             envPrefix:"INSTANCE_"`
	Email          EmailConfig    `yaml:"email"                envPrefix:"EMAIL_"`
	Database       DatabaseConfig `yaml:"database"             envPrefix:"DB_"`
	TorProxy       string         `yaml:"tor_proxy"            env:"TOR_PROXY"`
	CookieKey      string         `yaml:"cookie_key,omitempty" env:"COOKIE_KEY"`
	AuthReq        []string       `yaml:"auth_req"             env:"AUTH_REQ"        envSeparator:","`
	PostPerPage    int            `yaml:"posts_per_page"       env:"POSTS_PER_PAGE"                   envDefault:"10" `
	SupportedFiles []string       `yaml:"supported_files"      env:"SUPPORTED_FILES" envSeparator:"," envDefault:"image/gif,image/jpeg,image/png,image/webp,image/png,video/mp4,video/ogg,video/webm,audio/mpeg,audio/ogg,audio/wav,audio/wave,audio/x-wav"`
	ModKey         string         `yaml:"mod_key"              env:"MOD_KEY"`
}

var ConfigFile string
var configData AllConfig

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

	filePath, set := os.LookupEnv("CONFIG")
	err := env.Parse(&configData)
	if err != nil {
		log.Fatalln(err)
	}

	if set {
		loadFromYAML(filePath)
	} else {
		filePath = "config/config-init.yaml" // Default to this path

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if info, err := os.Stat("config/config-init"); !os.IsNotExist(err) {
				// If config/config-init.yaml doesn't exist, but config/config-init does, run migration
				migrateOldConfig()
				data, err := yaml.Marshal(&configData)
				if err != nil {
					Log.Println(err)
				}

				err = os.WriteFile(filePath, data, info.Mode())
				if err != nil {
					log.Println(err)
				}

				Log.Println("New config written to config/config-init.yaml")

			}
		} else if err == nil {
			loadFromYAML(filePath)
		}
	}
	ConfigFile = filePath

	// Instance configuration
	TP = configData.Instance.Protocol + "://"
	Domain = TP + configData.Instance.Domain
	Port = ":" + strconv.Itoa(configData.Instance.Port)
	InstanceName = configData.Instance.Name
	InstanceSummary = configData.Instance.Summary
	MaxFilesize = configData.Instance.MaxFilesize

	// Email configuration
	SiteEmail = configData.Email.Address
	SiteEmailPassword = configData.Email.Password
	SiteEmailServer = configData.Email.Host
	SiteEmailPort = strconv.Itoa(configData.Email.Port)
	SiteEmailNotifyTo = strings.Join(configData.Email.Notify, ",")

	// Database configuration
	DBHost = configData.Database.Host
	DBPort = configData.Database.Port
	DBName = configData.Database.Name
	DBUser = configData.Database.User
	DBPassword = configData.Database.Password

	// Other configuration
	TorProxy = configData.TorProxy
	CookieKey = configData.CookieKey
	AuthReq = configData.AuthReq
	PostCountPerPage = configData.PostPerPage
	SupportedFiles = configData.SupportedFiles
	Key = configData.ModKey

	Log.Println("Config loaded")
}

func loadFromYAML(filePath string) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		Log.Fatalln(err)
	}
	err = yaml.Unmarshal(file, &configData)
	if err != nil {
		Log.Fatalln(err)
	}
}

func migrateOldConfig() {
	Log.Println("Migrating config/config-init")

	configData.Instance.Port, _ = strconv.Atoi(getConfigValue("instanceport", "3000"))
	configData.Instance.Protocol = strings.TrimSuffix(getConfigValue("instancetp", "https"), "://")
	configData.Instance.Name = getConfigValue("instancename", "")
	configData.Instance.Summary = getConfigValue("instancesummary", "")
	configData.Instance.Salt = getConfigValue("instancesalt", "")

	configData.Email.Address = getConfigValue("emailaddress", "")
	configData.Email.Password = getConfigValue("emailpass", "")
	configData.Email.Host = getConfigValue("emailserver", "")
	configData.Email.Port, _ = strconv.Atoi(getConfigValue("emailport", "587"))
	configData.Email.Notify = strings.Split(getConfigValue("emailnotify", ""), ",")

	configData.Database.Host = getConfigValue("dbhost", "localhost")
	configData.Database.Port, _ = strconv.Atoi(getConfigValue("dbport", "5432"))
	configData.Database.Name = getConfigValue("dbname", "postgres")
	configData.Database.User = getConfigValue("dbuser", "postgres")
	configData.Database.Password = getConfigValue("dbpass", "")

	configData.TorProxy = getConfigValue("TorProxy", "127.0.0.1:9050")
	configData.CookieKey = getConfigValue("CookieKey", "")
	configData.ModKey = getConfigValue("ModKey", "")
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
