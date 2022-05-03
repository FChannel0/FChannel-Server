package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"math/rand"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/post"
	"github.com/FChannel0/FChannel-Server/routes"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html"

	_ "github.com/lib/pq"
)

func main() {

	Init()

	defer db.Close()
	defer db.CloseCache()

	// Routing and templates
	template := html.New("./views", ".html")
	template.Debug(true)

	TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName: "FChannel",
		Views:   template,
	})

	app.Use(logger.New())

	app.Static("/static", "./views")
	app.Static("/static", "./static")
	app.Static("/public", "./public")

	// Main actor
	app.Get("/", routes.Index)
	app.Get("/inbox", routes.Inbox)
	app.Get("/outbox", routes.Outbox)
	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)

	// Admin routes
	app.Post("/verify", routes.AdminVerify)
	app.Post("/auth", routes.AdminAuth)
	app.All("/"+config.Key+"/", routes.AdminIndex)
	app.Post("/"+config.Key+"/follow", routes.AdminFollow)
	app.Get("/"+config.Key+"/addboard", routes.AdminAddBoard)
	app.Get("/"+config.Key+"/postnews", routes.AdminPostNews)
	app.Get("/"+config.Key+"/newsdelete", routes.AdminNewsDelete)
	app.Post("/"+config.Key+"/:actor/follow", routes.AdminActorIndex)
	app.Get("/"+config.Key+"/:actor", routes.AdminActorIndex)
	app.Get("/news", routes.NewsGet)

	// Board managment
	app.Get("/banmedia", routes.BoardBanMedia)
	app.Get("/delete", routes.BoardDelete)
	app.Get("/deleteattach", routes.BoardDeleteAttach)
	app.Get("/marksensitive", routes.BoardMarkSensitive)
	app.Get("/remove", routes.BoardRemove)
	app.Get("/removeattach", routes.BoardRemoveAttach)
	app.Get("/addtoindex", routes.BoardAddToIndex)
	app.Get("/poparchive", routes.BoardPopArchive)
	app.Get("/autosubscribe", routes.BoardAutoSubscribe)
	app.Get("/blacklist", routes.BoardBlacklist)
	app.Get("/report", routes.BoardBlacklist)
	app.Get("/.well-known/webfinger", routes.Webfinger)
	app.Get("/api/media", routes.Media)

	// Board actor
	app.Get("/:actor/catalog", routes.CatalogGet)
	app.Post("/:actor/inbox", routes.ActorInbox)
	app.Post("/:actor/outbox", routes.ActorOutbox)
	app.Get("/:actor/following", routes.ActorFollowing)
	app.All("/:actor/followers", routes.ActorFollowers)
	app.Get("/:actor/reported", routes.ActorReported)
	app.Get("/:actor/archive", routes.ActorArchive)
	app.Get("/:actor", routes.OutboxGet)
	app.Post("/:actor", routes.ActorPost)
	app.Get("/:actor/:post", routes.PostGet)

	//404 handler
	app.Use(routes.NotFound)

	db.PrintAdminAuth()

	app.Listen(config.Port)
}

func Init() {
	var err error

	rand.Seed(time.Now().UnixNano())

	util.CreatedNeededDirectories()

	db.ConnectDB()

	db.InitCache()

	db.RunDatabaseSchema()

	go db.MakeCaptchas(100)

	config.Key = util.CreateKey(32)

	webfinger.FollowingBoards, err = activitypub.GetActorFollowingDB(config.Domain)

	if err != nil {
		panic(err)
	}

	go db.StartupArchive()

	go db.CheckInactive()

	webfinger.Boards, err = webfinger.GetBoardCollection()

	if err != nil {
		panic(err)
	}

	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if config.InstanceName != "" {
		if _, err = db.CreateNewBoardDB(*activitypub.CreateNewActor("", config.InstanceName, config.InstanceSummary, config.AuthReq, false)); err != nil {
			//panic(err)
		}

		if config.PublicIndexing == "true" {
			// TODO: comment out later
			//AddInstanceToIndex(config.Domain)
		}
	}

	// get list of themes
	themes, err := ioutil.ReadDir("./static/css/themes")
	if err != nil {
		panic(err)
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			config.Themes = append(config.Themes, strings.TrimSuffix(f.Name(), e))
		}
	}
}

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc("mod", func(i, j int) bool {
		return i%j == 0
	})

	engine.AddFunc("sub", func(i, j int) int {
		return i - j
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc("unixtoreadable", func(u int) string {
		return time.Unix(int64(u), 0).Format("Jan 02, 2006")
	})

	engine.AddFunc("timeToReadableLong", func(t time.Time) string {
		return t.Format("01/02/06(Mon)15:04:05")
	})

	engine.AddFunc("timeToUnix", func(t time.Time) string {
		return fmt.Sprint(t.Unix())
	})

	engine.AddFunc("proxy", util.MediaProxy)

	// previously short
	engine.AddFunc("shortURL", util.ShortURL)

	engine.AddFunc("parseAttachment", post.ParseAttachment)

	engine.AddFunc("parseContent", post.ParseContent)

	engine.AddFunc("shortImg", util.ShortImg)

	engine.AddFunc("convertSize", util.ConvertSize)

	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor, _ := webfinger.FingerActor(actorId)
		title := strings.ReplaceAll(post.ParseLinkTitle(actor.Id+"/", op, content), `/\&lt;`, ">")
		link := "<a href=\"/" + actor.Name + "/" + util.ShortURL(actor.Outbox, op) + "#" + util.ShortURL(actor.Outbox, id) + "\" title=\"" + title + "\" class=\"replyLink\">&gt;&gt;" + util.ShortURL(actor.Outbox, id) + "</a>"
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt", func(post activitypub.ObjectBase) string {
		var returnString string

		if post.Name != "" {
			returnString = post.Name + "| " + post.Content
		} else {
			returnString = post.Content
		}

		re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

		match := re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = match[0] + "..."
		}

		re = regexp.MustCompile(`(^.+\|)`)

		match = re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = strings.Replace(returnString, match[0], "<b>"+match[0]+"</b>", 1)
			returnString = strings.Replace(returnString, "|", ":", 1)
		}

		return returnString
	})
}
