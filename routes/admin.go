package routes

import (
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func AdminVerify(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin verify")
}

func AdminAuth(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin auth")
}

func AdminIndex(ctx *fiber.Ctx) error {
	actor, err := webfinger.GetActor(config.Domain)

	if err != nil {
		return err
	}

	follow, _ := webfinger.GetActorCollection(actor.Following)
	follower, _ := webfinger.GetActorCollection(actor.Followers)

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	var adminData AdminPage
	adminData.Following = following
	adminData.Followers = followers
	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Board.ModCred, _ = db.GetPasswordFromSession(ctx)
	adminData.Title = actor.Name + " Admin page"

	adminData.Boards = webfinger.Boards

	adminData.Board.Post.Actor = actor.Id

	adminData.PostBlacklist, _ = util.GetRegexBlacklistDB()

	adminData.Themes = &config.Themes

	return ctx.Render("admin", fiber.Map{
		"page": adminData,
	})
}

func AdminAddBoard(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin add board")
}

func AdminPostNews(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin post news")
}

func AdminNewsDelete(c *fiber.Ctx) error {
	// STUB

	return c.SendString("admin news delete")
}
