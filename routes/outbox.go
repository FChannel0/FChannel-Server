package routes

import (
	"strconv"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func Outbox(ctx *fiber.Ctx) error {
	// STUB

	return ctx.SendString("main outbox")
}

func OutboxGet(ctx *fiber.Ctx) error {

	actor := webfinger.GetActorByName(ctx.Params("actor"))

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		activitypub.GetActorInfo(ctx, actor.Id)
		return nil
	}

	collection, valid, err := wantToServePage(ctx.Params("actor"), 0)
	if err != nil {
		return err
	} else if !valid {
		// TODO: 404 template
		return ctx.SendString("404")
	}

	var page int
	postNum := ctx.Query("page")
	if postNum != "" {
		page, err = strconv.Atoi(postNum)
		if err != nil {
			return err
		}
	}

	var offset = 15
	var pages []int
	pageLimit := (float64(collection.TotalItems) / float64(offset))

	if pageLimit > 11 {
		pageLimit = 11
	}

	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.ModCred, _ = getPassword(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.CurrentPage = page
	data.ReturnTo = "feed"

	data.Board.Post.Actor = actor.Id

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	data.Board.Captcha = config.Domain + "/" + capt
	data.Board.CaptchaCode = util.GetCaptchaCode(data.Board.Captcha)

	data.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	data.Key = config.Key

	data.Boards = webfinger.Boards
	data.Posts = collection.OrderedItems

	data.Pages = pages
	data.TotalPage = len(data.Pages) - 1

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("nposts", fiber.Map{
		"page": data,
	}, "layouts/main")
}
