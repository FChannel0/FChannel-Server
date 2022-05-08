package main

import (
	"math/rand"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/routes"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/encryptcookie"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html"

	_ "github.com/lib/pq"
)

func main() {

	Init()

	defer db.Close()

	// Routing and templates
	template := html.New("./views", ".html")
	template.Debug(true)

	routes.TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName: "FChannel",
		Views:   template,
	})

	app.Use(logger.New())

	cookieKey, err := util.GetCookieKey()

	if err != nil {
		config.Log.Println(err)
	}

	app.Use(encryptcookie.New(encryptcookie.Config{
		Key: cookieKey,
	}))

	app.Static("/static", "./views")
	app.Static("/static", "./static")
	app.Static("/public", "./public")

	// Main actor
	app.Get("/", routes.Index)
	app.Post("/inbox", routes.Inbox)
	app.Post("/outbox", routes.Outbox)
	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)

	// Admin routes
	app.Post("/verify", routes.AdminVerify)
	app.Post("/auth", routes.AdminAuth)
	app.All("/"+config.Key+"/", routes.AdminIndex)
	app.Post("/"+config.Key+"/follow", routes.AdminFollow)
	app.Post("/"+config.Key+"/addboard", routes.AdminAddBoard)
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
	var actor activitypub.Actor
	var err error

	rand.Seed(time.Now().UnixNano())

	if err = util.CreatedNeededDirectories(); err != nil {
		config.Log.Println(err)
	}

	if err = db.Connect(); err != nil {
		config.Log.Println(err)
	}

	if err = db.RunDatabaseSchema(); err != nil {
		config.Log.Println(err)
	}

	if actor, err = activitypub.GetActorFromDB(config.Domain); err != nil {
		config.Log.Println(err)
	}

	if webfinger.FollowingBoards, err = actor.GetFollowing(); err != nil {
		config.Log.Println(err)
	}

	if webfinger.Boards, err = webfinger.GetBoardCollection(); err != nil {
		config.Log.Println(err)
	}

	if config.Key, err = util.CreateKey(32); err != nil {
		config.Log.Println(err)
	}

	if err = util.LoadThemes(); err != nil {
		config.Log.Println(err)
	}

	if err = db.InitInstance(); err != nil {
		config.Log.Println(err)
	}

	go webfinger.StartupArchive()

	go util.MakeCaptchas(100)

	go db.CheckInactive()

}
