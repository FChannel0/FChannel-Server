package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/route"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func NewsGet(ctx *fiber.Ctx) error {
	timestamp := 0

	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "NewsGet")
	}

	var data route.PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = webfinger.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.NewsItems = make([]db.NewsItem, 1)

	data.NewsItems[0], err = db.GetNewsItem(timestamp)
	if err != nil {
		return util.MakeError(err, "NewsGet")
	}

	data.Title = actor.PreferredUsername + ": " + data.NewsItems[0].Title

	data.Themes = &config.Themes
	data.ThemeCookie = route.GetThemeCookie(ctx)

	return ctx.Render("news", fiber.Map{"page": data}, "layouts/main")
}

func NewsGetAll(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "NewsGetAll")
	}

	var data route.PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Title = actor.PreferredUsername + " News"
	data.Boards = webfinger.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted

	data.NewsItems, err = db.GetNews(0)
	if err != nil {
		return util.MakeError(err, "NewsGetAll")
	}

	data.Themes = &config.Themes
	data.ThemeCookie = route.GetThemeCookie(ctx)

	return ctx.Render("anews", fiber.Map{"page": data}, "layouts/main")
}

// TODO routes/NewsPost
func NewsPost(c *fiber.Ctx) error {
	return c.SendString("admin post news")
}

// TODO routes/NewsDelete
func NewsDelete(c *fiber.Ctx) error {
	return c.SendString("admin news delete")
}
