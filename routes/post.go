package routes

import (
	"fmt"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"regexp"
)

func PostGet(ctx *fiber.Ctx) error {
	actor, err := db.GetActorByNameFromDB(ctx.Params("actor"))
	if err != nil {
		return err
	}

	postId := ctx.Params("post")

	inReplyTo := actor.Id + "/" + postId

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = getPassword(ctx)
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.ReturnTo = "feed"

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode = util.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Instance, err = db.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	returnData.Title = "/" + returnData.Board.Name + "/ - " + returnData.Board.PrefName

	returnData.Key = config.Key

	returnData.Boards = db.Boards

	re := regexp.MustCompile("f(\\w|[!@#$%^&*<>])+-(\\w|[!@#$%^&*<>])+")

	if re.MatchString(postId) { // if non local actor post
		name := util.GetActorFollowNameFromPath(postId)

		followActors, err := webfinger.GetActorsFollowFromName(actor, name)
		if err != nil {
			return err
		}

		followCollection, err := db.GetActorsFollowPostFromId(followActors, postId)
		if err != nil {
			return err
		}

		if len(followCollection.OrderedItems) > 0 {
			returnData.Board.InReplyTo = followCollection.OrderedItems[0].Id
			returnData.Posts = append(returnData.Posts, followCollection.OrderedItems[0])

			actor, err := webfinger.FingerActor(returnData.Board.InReplyTo)
			if err != nil {
				return err
			}

			returnData.Board.Post.Actor = actor.Id
		}
	} else {
		collection, err := db.GetObjectByIDFromDB(inReplyTo)
		if err != nil {
			return err
		}

		if collection.Actor != nil {
			returnData.Board.Post.Actor = collection.Actor.Id
			returnData.Board.InReplyTo = inReplyTo

			if len(collection.OrderedItems) > 0 {
				returnData.Posts = append(returnData.Posts, collection.OrderedItems[0])
			}
		}
	}

	if len(returnData.Posts) > 0 {
		returnData.PostId = util.ShortURL(returnData.Board.To, returnData.Posts[0].Id)
	}

	returnData.Themes = &config.Themes
	returnData.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("npost", fiber.Map{
		"page": returnData,
	}, "layouts/main")
}

func CatalogGet(ctx *fiber.Ctx) error {
	actorName := ctx.Params("actor")
	actor, err := db.GetActorByNameFromDB(actorName)
	if err != nil {
		return err
	}

	collection, err := db.GetObjectFromDBCatalog(actor.Id)

	fmt.Println(err)

	// TODO: implement this in template functions
	//	"showArchive": func() bool {
	//	col, err := db.GetActorCollectionDBTypeLimit(collection.Actor.Id, "Archive", 1)
	//	if err != nil {
	//		// TODO: figure out what to do here
	//		panic(err)
	//	}
	//
	//	if len(col.OrderedItems) > 0 {
	//		return true
	//	}
	//	return false
	//},

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = getPassword(ctx)
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Key = config.Key
	returnData.ReturnTo = "catalog"

	returnData.Board.Post.Actor = actor.Id

	returnData.Instance, err = db.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode = util.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = db.Boards

	returnData.Posts = collection.OrderedItems

	returnData.Themes = &config.Themes
	returnData.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("catalog", fiber.Map{
		"page": returnData,
	}, "layouts/main")
}
