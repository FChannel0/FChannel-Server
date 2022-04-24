package routes

import (
	"strconv"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func Outbox(ctx *fiber.Ctx) error {
	// STUB

	return ctx.SendString("main outbox")
}

func OutboxGet(ctx *fiber.Ctx) error {

	actor := db.GetActorByName(ctx.Params("actor"))

	if util.AcceptActivity(ctx.Get("Accept")) {
		db.GetActorInfo(ctx, actor.Id)
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

	var returnData PageData

	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.Summary = actor.Summary
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.ModCred, _ = getPassword(ctx)
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.CurrentPage = page
	returnData.ReturnTo = "feed"

	returnData.Board.Post.Actor = actor.Id

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode = util.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Key = config.Key

	returnData.Boards = db.Boards
	returnData.Posts = collection.OrderedItems

	var offset = 15
	var pages []int
	pageLimit := (float64(collection.TotalItems) / float64(offset))

	if pageLimit > 11 {
		pageLimit = 11
	}

	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	returnData.Pages = pages
	returnData.TotalPage = len(returnData.Pages) - 1

	returnData.Themes = &config.Themes
	returnData.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("nposts", fiber.Map{
		"page": returnData,
	}, "layouts/main")
}
