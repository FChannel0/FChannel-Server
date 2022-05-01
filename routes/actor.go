package routes

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/post"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func ActorInbox(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor inbox")
}

func ActorOutbox(ctx *fiber.Ctx) error {
	//var activity activitypub.Activity
	actor, err := webfinger.GetActorFromPath(ctx.Path(), "/")
	if err != nil {
		return err
	}

	contentType := util.GetContentType(ctx.Get("content-type"))

	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		hasCaptcha, err := db.BoardHasAuthType(actor.Name, "captcha")
		if err != nil {
			return err
		}

		valid, err := post.CheckCaptcha(ctx.FormValue("captcha"))
		if err == nil && hasCaptcha && valid {
			header, _ := ctx.FormFile("file")
			if header != nil {
				f, _ := header.Open()
				defer f.Close()
				if header.Size > (7 << 20) {
					ctx.Response().Header.SetStatusCode(403)
					_, err := ctx.Write([]byte("7MB max file size"))
					return err
				} else if isBanned, err := post.IsMediaBanned(f); err == nil && isBanned {
					//Todo add logging
					fmt.Println("media banned")
					ctx.Response().Header.SetStatusCode(403)
					_, err := ctx.Write([]byte("media banned"))
					return err
				} else if err != nil {
					return err
				}

				contentType, _ := util.GetFileContentType(f)

				if !post.SupportedMIMEType(contentType) {
					ctx.Response().Header.SetStatusCode(403)
					_, err := ctx.Write([]byte("file type not supported"))
					return err
				}
			}

			var nObj = activitypub.CreateObject("Note")
			nObj, err := post.ObjectFromForm(ctx, nObj)
			if err != nil {
				return err
			}

			nObj.Actor = config.Domain + "/" + actor.Name

			nObj, err = activitypub.WriteObjectToDB(nObj)
			if err != nil {
				return err
			}

			if len(nObj.To) == 0 {
				if err := db.ArchivePosts(actor); err != nil {
					return err
				}
			}

			activity, err := webfinger.CreateActivity("Create", nObj)
			if err != nil {
				return err
			}

			activity, err = webfinger.AddFollowersToActivity(activity)
			if err != nil {
				return err
			}

			go db.MakeActivityRequest(activity)

			var id string
			op := len(nObj.InReplyTo) - 1
			if op >= 0 {
				if nObj.InReplyTo[op].Id == "" {
					id = nObj.Id
				} else {
					id = nObj.InReplyTo[0].Id + "|" + nObj.Id
				}
			}

			ctx.Response().Header.Set("Status", "200")
			_, err = ctx.Write([]byte(id))
			return err
		}

		ctx.Response().Header.Set("Status", "403")
		_, err = ctx.Write([]byte("captcha could not auth"))
		return err
	} else { // json request
		activity, err := activitypub.GetActivityFromJson(ctx)
		if err != nil {
			return err
		}

		if res, err := activitypub.IsActivityLocal(activity); err == nil && res {
			if res := db.VerifyHeaderSignature(ctx, *activity.Actor); err == nil && !res {
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))
				return err
			}

			switch activity.Type {
			case "Create":
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))
				break

			case "Follow":
				var validActor bool
				var validLocalActor bool

				validActor = (activity.Object.Actor != "")
				validLocalActor = (activity.Actor.Id == actor.Id)

				var rActivity activitypub.Activity
				if validActor && validLocalActor {
					rActivity = db.AcceptFollow(activity)
					rActivity, err = db.SetActorFollowingDB(rActivity)
					if err != nil {
						return err
					}
					if err := db.MakeActivityRequest(activity); err != nil {
						return err
					}
				}

				webfinger.FollowingBoards, err = activitypub.GetActorFollowingDB(config.Domain)
				if err != nil {
					return err
				}

				webfinger.Boards, err = webfinger.GetBoardCollection()
				if err != nil {
					return err
				}
				break

			case "Delete":
				fmt.Println("This is a delete")
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
				break

			case "Note":
				ctx.Response().Header.Set("Satus", "403")
				_, err = ctx.Write([]byte("could not process activity"))
				break

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive

				actor, err := db.CreateNewBoardDB(*activitypub.CreateNewActor(name, prefname, summary, config.AuthReq, restricted))
				if err != nil {
					return err
				}

				if actor.Id != "" {
					var board []activitypub.ObjectBase
					var item activitypub.ObjectBase
					var removed bool = false

					item.Id = actor.Id
					for _, e := range webfinger.FollowingBoards {
						if e.Id != item.Id {
							board = append(board, e)
						} else {
							removed = true
						}
					}

					if !removed {
						board = append(board, item)
					}

					webfinger.FollowingBoards = board
					webfinger.Boards, err = webfinger.GetBoardCollection()
					return err
				}

				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))
				break

			default:
				ctx.Response().Header.Set("status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
			}
		} else if err != nil {
			return err
		} else {
			fmt.Println("is NOT activity")
			ctx.Response().Header.Set("Status", "403")
			_, err = ctx.Write([]byte("could not process activity"))
			return err
		}
	}

	return nil
}

func ActorFollowing(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor following")
}

func ActorFollowers(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor followers")
}

func ActorReported(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor reported")
}

func ActorArchive(c *fiber.Ctx) error {
	// STUB

	return c.SendString("actor archive")
}

func ActorPost(ctx *fiber.Ctx) error {
	header, _ := ctx.FormFile("file")

	if ctx.FormValue("inReplyTo") == "" && header == nil {
		return ctx.Render("403", fiber.Map{
			"message": "Media is required for new posts",
		})
	}

	var file multipart.File

	if header != nil {
		file, _ = header.Open()
	}

	if file != nil && header.Size > (7<<20) {
		return ctx.Render("403", fiber.Map{
			"message": "7MB max file size",
		})
	}

	if is, _ := util.IsPostBlacklist(ctx.FormValue("comment")); is {
		errors.New("\n\nBlacklist post blocked\n\n")
		return ctx.Redirect("/", 301)
	}

	if ctx.FormValue("inReplyTo") == "" || file == nil {
		if ctx.FormValue("comment") == "" && ctx.FormValue("subject") == "" {
			return ctx.Render("403", fiber.Map{
				"message": "Comment or Subject required",
			})
		}
	}

	if len(ctx.FormValue("comment")) > 2000 {
		return ctx.Render("403", fiber.Map{
			"message": "Comment limit 2000 characters",
		})
	}

	if len(ctx.FormValue("subject")) > 100 || len(ctx.FormValue("name")) > 100 || len(ctx.FormValue("options")) > 100 {
		return ctx.Render("403", fiber.Map{
			"message": "Name, Subject or Options limit 100 characters",
		})
	}

	if ctx.FormValue("captcha") == "" {
		return ctx.Render("403", fiber.Map{
			"message": "Incorrect Captcha",
		})
	}

	b := bytes.Buffer{}
	we := multipart.NewWriter(&b)

	if file != nil {
		var fw io.Writer

		fw, err := we.CreateFormFile("file", header.Filename)

		if err != nil {
			errors.New("error with form file create")
		}
		_, err = io.Copy(fw, file)

		if err != nil {
			errors.New("error with form file copy")
		}
	}

	reply, _ := post.ParseCommentForReply(ctx.FormValue("comment"))

	form, _ := ctx.MultipartForm()

	for key, r0 := range form.Value {
		if key == "captcha" {
			err := we.WriteField(key, ctx.FormValue("captchaCode")+":"+ctx.FormValue("captcha"))
			if err != nil {
				errors.New("error with writing captcha field")
			}
		} else if key == "name" {
			name, tripcode, _ := post.CreateNameTripCode(ctx)

			err := we.WriteField(key, name)
			if err != nil {
				errors.New("error with writing name field")
			}

			err = we.WriteField("tripcode", tripcode)
			if err != nil {
				errors.New("error with writing tripcode field")
			}
		} else {
			err := we.WriteField(key, r0[0])
			if err != nil {
				errors.New("error with writing field")
			}
		}
	}

	if ctx.FormValue("inReplyTo") == "" && reply != "" {
		err := we.WriteField("inReplyTo", reply)
		if err != nil {
			errors.New("error with writing inReplyTo field")
		}
	}

	we.Close()

	sendTo := ctx.FormValue("sendTo")

	req, err := http.NewRequest("POST", sendTo, &b)

	if err != nil {
		errors.New("error with post form req")
	}

	req.Header.Set("Content-Type", we.FormDataContentType())

	resp, err := util.RouteProxy(req)

	if err != nil {
		errors.New("error with post form resp")
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode == 200 {

		var obj activitypub.ObjectBase

		obj = post.ParseOptions(ctx, obj)
		for _, e := range obj.Option {
			if e == "noko" || e == "nokosage" {
				return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName")+"/"+util.ShortURL(ctx.FormValue("sendTo"), string(body)), 301)
			}
		}

		if ctx.FormValue("returnTo") == "catalog" {
			return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName")+"/catalog", 301)
		} else {
			return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName"), 301)
		}
	}

	if resp.StatusCode == 403 {
		return ctx.Render("403", fiber.Map{
			"message": string(body),
		})
	}

	return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName"), 301)
}
