package routes

import (
	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func ArchiveGet(ctx *fiber.Ctx) error {
	// TODO
	collection := ctx.Locals("collection").(activitypub.Collection)
	actor := collection.Actor

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = *actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = db.GetPasswordFromSession(ctx)
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Key = config.Key
	returnData.ReturnTo = "archive"

	returnData.Board.Post.Actor = actor.Id

	var err error
	returnData.Instance, err = activitypub.GetActorFromDB(config.Domain)

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode = util.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = webfinger.Boards

	returnData.Posts = collection.OrderedItems

	returnData.Themes = &config.Themes
	returnData.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("archive", fiber.Map{
		"page": returnData,
	}, "layouts/main")
}
