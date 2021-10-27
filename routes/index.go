package routes

import (
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/gofiber/fiber/v2"
)

func Index(c *fiber.Ctx) error {
	actor, err := db.GetActor(config.Domain)
	if err != nil {
		return err
	}

	var data PageData
	data.Title = "Welcome to " + actor.PreferredUsername
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = Boards
	data.Board.Name = ""
	data.Key = *Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = GetPasswordFromCtx(c)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	//almost certainly there is a better algorithm for this but the old one was wrong
	//and I suck at math. This works at least.
	data.BoardRemainer = make([]int, 3-(len(data.Boards)%3))
	if len(data.BoardRemainer) == 3 {
		data.BoardRemainer = make([]int, 0)
	}

	col := GetCollectionFromReq("https://fchan.xyz/followers")

	if len(col.Items) > 0 {
		data.InstanceIndex = col.Items
	}

	data.NewsItems, err = db.GetNewsFromDB(3)
	if err != nil {
		return err
	}

	data.Themes = &Themes

	data.ThemeCookie = getThemeCookie(c)

	return c.Render("index", fiber.Map{
		"page": data,
	}, "layouts/main")
}
