package routes

import (
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func Index(ctx *fiber.Ctx) error {
	actor, err := db.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	var data PageData
	data.Title = "Welcome to " + actor.PreferredUsername
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = db.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = getPassword(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	//almost certainly there is a better algorithm for this but the old one was wrong
	//and I suck at math. This works at least.
	data.BoardRemainer = make([]int, 3-(len(data.Boards)%3))
	if len(data.BoardRemainer) == 3 {
		data.BoardRemainer = make([]int, 0)
	}

	col, err := webfinger.GetCollectionFromReq("https://fchan.xyz/followers")
	if err != nil {
		return err
	}

	if len(col.Items) > 0 {
		data.InstanceIndex = col.Items
	}

	data.NewsItems, err = db.GetNewsFromDB(3)
	if err != nil {
		return err
	}

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("index", fiber.Map{
		"page": data,
	}, "layouts/main")
}
