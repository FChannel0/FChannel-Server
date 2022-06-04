package routes

import (
	"errors"
	"net/http"
	"os"
	"regexp"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/post"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func BoardBanMedia(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		err = errors.New("missing postID or auth")
		return util.MakeError(err, "BoardBanMedia")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.MakeError(err, "BoardBanMedia")
	}

	if len(col.OrderedItems) == 0 {
		err = errors.New("no collection")
		return util.MakeError(err, "BoardBanMedia")
	}

	if len(col.OrderedItems[0].Attachment) == 0 {
		err = errors.New("no attachment")
		return util.MakeError(err, "BoardBanMedia")
	}

	var actor activitypub.Actor
	actor.Id = col.OrderedItems[0].Actor

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		err = errors.New("actor does not have auth")
		return util.MakeError(err, "BoardBanMedia")
	}

	re := regexp.MustCompile(config.Domain)
	file := re.ReplaceAllString(col.OrderedItems[0].Attachment[0].Href, "")

	f, err := os.Open("." + file)

	if err != nil {
		return util.MakeError(err, "BoardBanMedia")
	}

	defer f.Close()

	bytes := make([]byte, 2048)

	if _, err = f.Read(bytes); err != nil {
		return util.MakeError(err, "BoardBanMedia")
	}

	if banned, err := post.IsMediaBanned(f); err == nil && !banned {
		query := `insert into bannedmedia (hash) values ($1)`
		if _, err := config.DB.Exec(query, util.HashBytes(bytes)); err != nil {
			return util.MakeError(err, "BoardBanMedia")
		}
	}

	var isOP bool
	var local bool
	var obj activitypub.ObjectBase
	obj.Id = postID
	obj.Actor = actor.Id

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return util.MakeError(err, "BoardBanMedia")
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return util.MakeError(err, "BoardBanMedia")
		}
	}

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return util.MakeError(err, "BoardBanMedia")
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return util.MakeError(err, "BoardBanMedia")
	}

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDelete(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		err = errors.New("missing postID or auth")
		return util.MakeError(err, "BoardDelete")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.MakeError(err, "BoardDelete")
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		err = errors.New("actor does not have auth")
		return util.MakeError(err, "BoardDelete")
	}

	var isOP bool
	obj := activitypub.ObjectBase{Id: postID}

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	}

	var local bool

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return util.MakeError(err, "BoardDelete")
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return util.MakeError(err, "BoardDelete")
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDeleteAttach(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		err = errors.New("missing postID or auth")
		return util.MakeError(err, "BoardDeleteAttach")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.MakeError(err, "BoardDeleteAttach")
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.MakeError(err, "BoardDeleteAttach")
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.MakeError(err, "BoardDeleteAttach")
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.MakeError(err, "BoardDeleteAttach")
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.MakeError(err, "BoardDeleteAttach")
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.MakeError(err, "BoardDeleteAttach")
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	} else if local, _ := obj.IsLocal(); !local && OP != "" {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else if OP != "" {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardMarkSensitive(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		err = errors.New("missing postID or auth")
		return util.MakeError(err, "BoardMarkSensitive")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.MakeError(err, "BoardMarkSensitive")
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.MakeError(err, "BoardMarkSensitive")
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		err = errors.New("actor does not have auth")
		return util.MakeError(err, "BoardMarkSensitive")
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err = obj.MarkSensitive(true); err != nil {
		return util.MakeError(err, "BoardMarkSensitive")
	}

	if isOP, _ := obj.CheckIfOP(); !isOP && OP != "" {
		if local, _ := obj.IsLocal(); !local {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		}

		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

// TODO routes/BoardRemove
func BoardRemove(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove")
}

// TODO routes/BoardAddToIndex
func BoardAddToIndex(ctx *fiber.Ctx) error {
	return ctx.SendString("board add to index")
}

// TODO routes/BoardPopArchive
func BoardPopArchive(ctx *fiber.Ctx) error {
	return ctx.SendString("board pop archive")
}

// TODO routes/BoardAutoSubscribe
func BoardAutoSubscribe(ctx *fiber.Ctx) error {
	return ctx.SendString("board auto subscribe")
}

// TODO routes/BoardBlacklist
func BoardBlacklist(ctx *fiber.Ctx) error {
	return ctx.SendString("board blacklist")
}

func BoardReport(ctx *fiber.Ctx) error {
	id := ctx.FormValue("id")
	board := ctx.FormValue("board")
	reason := ctx.FormValue("comment")
	close := ctx.FormValue("close")

	actor, err := activitypub.GetActorByNameFromDB(board)

	if err != nil {
		return util.MakeError(err, "BoardReport")
	}
	_, auth := util.GetPasswordFromSession(ctx)

	var captcha = ctx.FormValue("captchaCode") + ":" + ctx.FormValue("captcha")

	if len(reason) > 100 {
		return ctx.Status(403).Render("403", fiber.Map{
			"message": "Report comment limit 100 characters",
		})
	}

	if ok, _ := post.CheckCaptcha(captcha); !ok && close != "1" {
		return ctx.Status(403).Render("403", fiber.Map{
			"message": "Invalid captcha",
		})
	}

	var obj = activitypub.ObjectBase{Id: id}

	if close == "1" {
		if auth, err := util.HasAuth(auth, actor.Id); !auth {
			config.Log.Println(err)
			return ctx.Status(404).Render("404", fiber.Map{
				"message": "Something broke",
			})
		}

		if local, _ := obj.IsLocal(); !local {
			if err := db.CloseLocalReport(obj.Id, board); err != nil {
				config.Log.Println(err)
				return ctx.Status(404).Render("404", fiber.Map{
					"message": "Something broke",
				})
			}

			return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
		}

		if err := obj.DeleteReported(); err != nil {
			config.Log.Println(err)
			return ctx.Status(404).Render("404", fiber.Map{
				"message": "Something broke",
			})
		}

		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if local, _ := obj.IsLocal(); !local {
		if err := db.CreateLocalReport(id, board, reason); err != nil {
			config.Log.Println(err)
			return ctx.Status(404).Render("404", fiber.Map{
				"message": "Something broke",
			})
		}

		return ctx.Redirect("/"+board+"/"+util.RemoteShort(obj.Id), http.StatusSeeOther)
	}

	if err := db.CreateLocalReport(obj.Id, board, reason); err != nil {
		config.Log.Println(err)
		return ctx.Status(404).Render("404", fiber.Map{
			"message": "Something broke",
		})
	}

	return ctx.Redirect(id, http.StatusSeeOther)
}
