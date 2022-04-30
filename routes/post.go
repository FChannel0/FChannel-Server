package routes

import (
	"regexp"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func PostGet(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))
	if err != nil {
		return err
	}

	postId := ctx.Params("post")

	inReplyTo := actor.Id + "/" + postId

	var data PageData

	re := regexp.MustCompile("f(\\w|[!@#$%^&*<>])+-(\\w|[!@#$%^&*<>])+")

	if re.MatchString(postId) { // if non local actor post
		name := activitypub.GetActorFollowNameFromPath(postId)

		followActors, err := webfinger.GetActorsFollowFromName(actor, name)
		if err != nil {
			return err
		}

		followCollection, err := activitypub.GetActorsFollowPostFromId(followActors, postId)
		if err != nil {
			return err
		}

		if len(followCollection.OrderedItems) > 0 {
			data.Board.InReplyTo = followCollection.OrderedItems[0].Id
			data.Posts = append(data.Posts, followCollection.OrderedItems[0])

			actor, err := webfinger.FingerActor(data.Board.InReplyTo)
			if err != nil {
				return err
			}

			data.Board.Post.Actor = actor.Id
		}
	} else {
		collection, err := activitypub.GetObjectByIDFromDB(inReplyTo)
		if err != nil {
			return err
		}

		if collection.Actor != nil {
			data.Board.Post.Actor = collection.Actor.Id
			data.Board.InReplyTo = inReplyTo

			if len(collection.OrderedItems) > 0 {
				data.Posts = append(data.Posts, collection.OrderedItems[0])
			}
		}
	}

	if len(data.Posts) > 0 {
		data.PostId = util.ShortURL(data.Board.To, data.Posts[0].Id)
	}

	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.ModCred, _ = getPassword(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.ReturnTo = "feed"

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}
	data.Board.Captcha = config.Domain + "/" + capt
	data.Board.CaptchaCode = util.GetCaptchaCode(data.Board.Captcha)

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	data.Key = config.Key
	data.Boards = webfinger.Boards

	data.Title = "/" + data.Board.Name + "/ - " + data.PostId

	if len(data.Posts) > 0 {
		data.Meta.Description = data.Posts[0].Content
		data.Meta.Url = data.Posts[0].Id
		data.Meta.Title = data.Posts[0].Name
		data.Meta.Preview = data.Posts[0].Preview.Href
	}

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("npost", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func CatalogGet(ctx *fiber.Ctx) error {
	actorName := ctx.Params("actor")
	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return err
	}

	collection, err := activitypub.GetObjectFromDBCatalog(actor.Id)

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

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.ModCred, _ = getPassword(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Key = config.Key
	data.ReturnTo = "catalog"

	data.Board.Post.Actor = actor.Id

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	capt, err := db.GetRandomCaptcha()
	if err != nil {
		return err
	}

	data.Board.Captcha = config.Domain + "/" + capt
	data.Board.CaptchaCode = util.GetCaptchaCode(data.Board.Captcha)

	data.Title = "/" + data.Board.Name + "/ - catalog"

	data.Boards = webfinger.Boards
	data.Posts = collection.OrderedItems

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = getThemeCookie(ctx)

	return ctx.Render("catalog", fiber.Map{
		"page": data,
	}, "layouts/main")
}
