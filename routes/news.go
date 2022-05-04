package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func NewsGet(ctx *fiber.Ctx) error {
	// TODO
	timestamp := 0

	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = webfinger.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = db.GetPassword(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.NewsItems = make([]db.NewsItem, 1)

	data.NewsItems[0], err = db.GetNewsItemFromDB(timestamp)
	if err != nil {
		return err
	}

	data.Title = actor.PreferredUsername + ": " + data.NewsItems[0].Title

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("news", fiber.Map{"page": data}, "layouts/main")
}

func AllNewsGet(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Title = actor.PreferredUsername + " News"
	data.Boards = webfinger.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = db.GetPassword(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted

	data.NewsItems, err = db.GetNewsFromDB(0)
	if err != nil {
		return err
	}

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("anews", fiber.Map{"page": data}, "layouts/main")
}
