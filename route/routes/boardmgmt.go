package routes

import (
	"net/http"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func BoardBanMedia(ctx *fiber.Ctx) error {
	return ctx.SendString("board ban media")
}

func BoardDelete(ctx *fiber.Ctx) error {
	id := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if id == "" || auth == "" {
		ctx.Response().Header.SetStatusCode(http.StatusBadRequest)

		_, err := ctx.Write([]byte("id or auth empty"))
		return util.MakeError(err, "BoardDelete")
	}

	activity := activitypub.Activity{Id: id}
	col, err := activity.GetCollection()

	if err != nil {
		return util.MakeError(err, "BoardDelete")
	}

	if len(col.OrderedItems) < 1 {
		actor, err := activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.MakeError(err, "BoardDelete")
		}

		if has, _ := util.HasAuth(auth, actor.Id); !has {
			ctx.Response().Header.SetStatusCode(http.StatusBadRequest)

			_, err := ctx.Write([]byte("does not have auth"))
			return util.MakeError(err, "BoardDelete")
		}

		obj := activitypub.ObjectBase{Id: id}
		isOP, _ := obj.CheckIfOP()

		if !isOP {
			if err := obj.Tombstone(); err != nil {
				return util.MakeError(err, "BoardDelete")
			}
		} else {
			if err := obj.TombstoneReplies(); err != nil {
				return util.MakeError(err, "BoardDelete")
			}
		}

		if err := actor.UnArchiveLast(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}

		if ctx.Query("manage") == "t" {
			return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
		}

		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}

	actorID := col.OrderedItems[0].Actor

	if has, _ := util.HasAuth(auth, actorID); !has {
		ctx.Response().Header.SetStatusCode(http.StatusBadRequest)

		_, err := ctx.Write([]byte("does not have auth"))
		return util.MakeError(err, "BoardDelete")
	}

	var obj activitypub.ObjectBase
	obj.Id = id
	obj.Actor = actorID

	isOP, _ := obj.CheckIfOP()

	var OP string

	if len(col.OrderedItems[0].InReplyTo) > 0 {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	}

	if !isOP {
		if err := obj.Tombstone(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	}

	if local, _ := obj.IsLocal(); !local {
		if err := obj.DeleteRequest(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	}

	actor := activitypub.Actor{Id: actorID}
	err = actor.UnArchiveLast()

	if err != nil {
		return util.MakeError(err, "BoardDelete")
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if !isOP {
		if local, _ := obj.IsLocal(); !local {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDeleteAttach(ctx *fiber.Ctx) error {
	return ctx.SendString("board delete attach")
}

func BoardMarkSensitive(ctx *fiber.Ctx) error {
	return ctx.SendString("board mark sensitive")
}

func BoardRemove(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove")
}

func BoardRemoveAttach(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove attach")
}

func BoardAddToIndex(ctx *fiber.Ctx) error {
	return ctx.SendString("board add to index")
}

func BoardPopArchive(ctx *fiber.Ctx) error {
	return ctx.SendString("board pop archive")
}

func BoardAutoSubscribe(ctx *fiber.Ctx) error {
	return ctx.SendString("board auto subscribe")
}

func BoardBlacklist(ctx *fiber.Ctx) error {
	return ctx.SendString("board blacklist")
}

func BoardReport(ctx *fiber.Ctx) error {
	return ctx.SendString("board report")
}
