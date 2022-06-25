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

func Index(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "Index")
	}

	// this is a activitpub json request return json instead of html page
	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetInfoResp(ctx)
		return nil
	}

	var data route.PageData

	data.NewsItems, err = db.GetNews(3)
	if err != nil {
		return util.MakeError(err, "Index")
	}

	data.Title = "Welcome to " + actor.PreferredUsername
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = webfinger.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	//almost certainly there is a better algorithm for this but the old one was wrong
	//and I suck at math. This works at least.
	data.BoardRemainer = make([]int, 3-(len(data.Boards)%3))

	if len(data.BoardRemainer) == 3 {
		data.BoardRemainer = make([]int, 0)
	}

	data.Meta.Description = data.PreferredUsername + " a federated image board based on ActivityPub. The current version of the code running on the server is still a work-in-progress product, expect a bumpy ride for the time being. Get the server code here: https://github.com/FChannel0."
	data.Meta.Url = data.Board.Domain
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = route.GetThemeCookie(ctx)

	return ctx.Render("index", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func Inbox(ctx *fiber.Ctx) error {
	// TODO main actor Inbox route
	return ctx.SendString("main inbox")
}

func Outbox(ctx *fiber.Ctx) error {
	actor, err := webfinger.GetActorFromPath(ctx.Path(), "/")

	if err != nil {
		return util.MakeError(err, "Outbox")
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetOutbox(ctx)
		return nil
	}

	return route.ParseOutboxRequest(ctx, actor)
}

func Following(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowingResp(ctx)
}

func Followers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowersResp(ctx)
}
