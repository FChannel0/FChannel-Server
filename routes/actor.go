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

func ActorInbox(ctx *fiber.Ctx) error {
	activity, err := activitypub.GetActivityFromJson(ctx)
	if err != nil {
		return err
	}

	if activity.Actor.PublicKey.Id == "" {
		nActor, err := webfinger.FingerActor(activity.Actor.Id)
		if err != nil {
			return err
		}

		activity.Actor = &nActor
	}

	if !db.VerifyHeaderSignature(ctx, *activity.Actor) {
		response := activitypub.RejectActivity(activity)
		return db.MakeActivityRequest(response)
	}

	switch activity.Type {
	case "Create":
		for _, e := range activity.To {
			if res, err := activitypub.IsActorLocal(e); err == nil && res {
				if res, err := activitypub.IsActorLocal(activity.Actor.Id); err == nil && res {
					col, err := activity.Object.GetCollection()
					if err != nil {
						return err
					}

					if len(col.OrderedItems) < 1 {
						break
					}

					if err := activity.Object.WriteCache(); err != nil {
						return err
					}

					actor, err := activitypub.GetActorFromDB(e)
					if err != nil {
						return err
					}

					if err := db.ArchivePosts(actor); err != nil {
						return err
					}

					//SendToFollowers(e, activity)
				} else if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}

		break

	case "Delete":
		for _, e := range activity.To {
			actor, err := activitypub.GetActorFromDB(e)
			if err != nil {
				return err
			}

			if actor.Id != "" && actor.Id != config.Domain {
				if activity.Object.Replies.OrderedItems != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						if err := k.Tombstone(); err != nil {
							return err
						}
					}
				}

				if err := activity.Object.Tombstone(); err != nil {
					return err
				}
				if err := actor.UnArchiveLast(); err != nil {
					return err
				}
				break
			}
		}
		break

	case "Follow":
		for _, e := range activity.To {
			if res, err := activitypub.GetActorFromDB(e); err == nil && res.Id != "" {
				response := db.AcceptFollow(activity)
				response, err := activitypub.SetActorFollowerDB(response)
				if err != nil {
					return err
				}

				if err := db.MakeActivityRequest(response); err != nil {
					return err
				}

				alreadyFollow := false
				alreadyFollowing := false
				autoSub, err := response.Actor.GetAutoSubscribe()
				if err != nil {
					return err
				}

				following, err := response.Actor.GetFollowing()
				if err != nil {
					return err
				}

				for _, e := range following {
					if e.Id == response.Object.Id {
						alreadyFollow = true
					}
				}

				actor, err := webfinger.FingerActor(response.Object.Actor)
				if err != nil {
					return err
				}

				remoteActorFollowingCol, err := webfinger.GetCollectionFromReq(actor.Following)
				if err != nil {
					return err
				}

				for _, e := range remoteActorFollowingCol.Items {
					if e.Id == response.Actor.Id {
						alreadyFollowing = true
					}
				}

				if autoSub && !alreadyFollow && alreadyFollowing {
					followActivity, err := db.MakeFollowActivity(response.Actor.Id, response.Object.Actor)
					if err != nil {
						return err
					}

					if res, err := webfinger.FingerActor(response.Object.Actor); err == nil && res.Id != "" {
						if err := db.MakeActivityRequestOutbox(followActivity); err != nil {
							return err
						}
					} else if err != nil {
						return err
					}
				}
			} else if err != nil {
				return err
			} else {
				fmt.Println("follow request for rejected")
				response := activitypub.RejectActivity(activity)
				return db.MakeActivityRequest(response)
			}
		}
		break

	case "Reject":
		if activity.Object.Object.Type == "Follow" {
			fmt.Println("follow rejected")
			if _, err := db.SetActorFollowingDB(activity); err != nil {
				return err
			}
		}
		break
	}

	return nil
}

func ActorOutbox(ctx *fiber.Ctx) error {
	//var activity activitypub.Activity
	actor, err := webfinger.GetActorFromPath(ctx.Path(), "/")
	if err != nil {
		return err
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetOutbox(ctx)
		return nil
	}

	return ParseOutboxRequest(ctx, actor)
}

func ActorFollowing(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowingResp(ctx)
}

func ActorFollowers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowersResp(ctx)
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
