package routes

import (
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/post"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
)

var ErrorPageLimit = errors.New("above page limit")

func getThemeCookie(c *fiber.Ctx) string {
	cookie := c.Cookies("theme")
	if cookie != "" {
		cookies := strings.SplitN(cookie, "=", 2)
		return cookies[0]
	}

	return "default"
}

func wantToServePage(actorName string, page int) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	// TODO: don't hard code?
	if page > 10 {
		return collection, serve, ErrorPageLimit
	}

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = actor.GetCollectionPage(page)
		if err != nil {
			return collection, false, err
		}

		collection.Actor = actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func wantToServeCatalog(actorName string) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = actor.GetCatalogCollection()
		if err != nil {
			return collection, false, err
		}

		collection.Actor = actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func wantToServeArchive(actorName string) (activitypub.Collection, bool, error) {
	var collection activitypub.Collection
	serve := false

	actor, err := activitypub.GetActorByNameFromDB(actorName)
	if err != nil {
		return collection, false, err
	}

	if actor.Id != "" {
		collection, err = actor.GetCollectionType("Archive")
		if err != nil {
			return collection, false, err
		}

		collection.Actor = actor
		return collection, true, nil
	}

	return collection, serve, nil
}

func ParseOutboxRequest(ctx *fiber.Ctx, actor activitypub.Actor) error {
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

			nObj, err = nObj.Write()
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

				actor, _ := activitypub.GetActorFromDB(config.Domain)
				webfinger.FollowingBoards, err = actor.GetFollowing()
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

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc("mod", func(i, j int) bool {
		return i%j == 0
	})

	engine.AddFunc("sub", func(i, j int) int {
		return i - j
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc("unixtoreadable", func(u int) string {
		return time.Unix(int64(u), 0).Format("Jan 02, 2006")
	})

	engine.AddFunc("timeToReadableLong", func(t time.Time) string {
		return t.Format("01/02/06(Mon)15:04:05")
	})

	engine.AddFunc("timeToUnix", func(t time.Time) string {
		return fmt.Sprint(t.Unix())
	})

	engine.AddFunc("proxy", util.MediaProxy)

	// previously short
	engine.AddFunc("shortURL", util.ShortURL)

	engine.AddFunc("parseAttachment", post.ParseAttachment)

	engine.AddFunc("parseContent", post.ParseContent)

	engine.AddFunc("shortImg", util.ShortImg)

	engine.AddFunc("convertSize", util.ConvertSize)

	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor, _ := webfinger.FingerActor(actorId)
		title := strings.ReplaceAll(post.ParseLinkTitle(actor.Id+"/", op, content), `/\&lt;`, ">")
		link := "<a href=\"/" + actor.Name + "/" + util.ShortURL(actor.Outbox, op) + "#" + util.ShortURL(actor.Outbox, id) + "\" title=\"" + title + "\" class=\"replyLink\">&gt;&gt;" + util.ShortURL(actor.Outbox, id) + "</a>"
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt", func(post activitypub.ObjectBase) string {
		var returnString string

		if post.Name != "" {
			returnString = post.Name + "| " + post.Content
		} else {
			returnString = post.Content
		}

		re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

		match := re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = match[0] + "..."
		}

		re = regexp.MustCompile(`(^.+\|)`)

		match = re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = strings.Replace(returnString, match[0], "<b>"+match[0]+"</b>", 1)
			returnString = strings.Replace(returnString, "|", ":", 1)
		}

		return returnString
	})
}
