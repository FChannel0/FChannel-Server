package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/post"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
)

func ParseOutboxRequest(ctx *fiber.Ctx) error {
	//var activity activitypub.Activity

	actor, err := webfinger.GetActorFromPath(ctx.Path(), "/")
	if err != nil {
		return err
	}

	contentType := GetContentType(ctx.Get("content-type"))

	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {

		hasCaptcha, err := db.BoardHasAuthType(actor.Name, "captcha")
		if err != nil {
			return err
		}

		valid, err := CheckCaptcha(ctx.FormValue("captcha"))
		if err == nil && hasCaptcha && valid {
			header, _ := ctx.FormFile("file")

			if header != nil {
				f, _ := header.Open()
				defer f.Close()
				if header.Size > (7 << 20) {
					return ctx.Render("403", fiber.Map{
						"message": "7MB max file size",
					})
				} else if res, err := IsMediaBanned(f); err == nil && res {
					//Todo add logging
					fmt.Println("media banned")
					return ctx.Redirect("/", 301)
				} else if err != nil {
					return err
				}

				contentType, _ := post.GetFileContentType(f)

				if !SupportedMIMEType(contentType) {
					return ctx.Render("403", fiber.Map{
						"message": "file type not supported",
					})
				}
			}

			var nObj = activitypub.CreateObject("Note")
			nObj, err := ObjectFromForm(ctx, nObj)
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

			activity, err := CreateActivity("Create", nObj)
			if err != nil {
				return err
			}

			activity, err = AddFollowersToActivity(activity)
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

			ctx.Response().Header.Add("status", "200")
			_, err = ctx.Write([]byte(id))
			return err
		}

		ctx.Response().Header.Add("status", "403")
		_, err = ctx.Write([]byte("captcha could not auth"))
		return err
	} else {
		activity, err := activitypub.GetActivityFromJson(ctx)
		if err != nil {
			return err
		}

		if res, err := activitypub.IsActivityLocal(activity); err == nil && res {
			if res := db.VerifyHeaderSignature(ctx, *activity.Actor); err == nil && !res {
				ctx.Response().Header.Add("status", "403")
				_, err = ctx.Write([]byte(""))
				return err
			}

			switch activity.Type {
			case "Create":
				ctx.Response().Header.Add("status", "403")
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
				ctx.Response().Header.Add("status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
				break

			case "Note":
				ctx.Response().Header.Add("status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
				break

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive

				actor, err := db.CreateNewBoardDB(*activitypub.CreateNewActor(name, prefname, summary, authReq, restricted))
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

				ctx.Response().Header.Add("status", "403")
				_, err = ctx.Write([]byte(""))
				break

			default:
				ctx.Response().Header.Add("status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
			}
		} else if err != nil {
			return err
		} else {
			fmt.Println("is NOT activity")
			ctx.Response().Header.Add("status", "403")
			_, err = ctx.Write([]byte("could not process activity"))
			return err
		}
	}

	return nil
}

func ObjectFromForm(ctx *fiber.Ctx, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	header, _ := ctx.FormFile("file")
	file, _ := header.Open()
	var err error

	if file != nil {
		defer file.Close()

		var tempFile = new(os.File)
		obj.Attachment, tempFile, err = activitypub.CreateAttachmentObject(file, header)
		if err != nil {
			return obj, err
		}

		defer tempFile.Close()

		fileBytes, _ := ioutil.ReadAll(file)

		tempFile.Write(fileBytes)

		re := regexp.MustCompile(`image/(jpe?g|png|webp)`)
		if re.MatchString(obj.Attachment[0].MediaType) {
			fileLoc := strings.ReplaceAll(obj.Attachment[0].Href, config.Domain, "")

			cmd := exec.Command("exiv2", "rm", "."+fileLoc)

			if err := cmd.Run(); err != nil {
				return obj, err
			}
		}

		obj.Preview = activitypub.CreatePreviewObject(obj.Attachment[0])
	}

	obj.AttributedTo = util.EscapeString(ctx.FormValue("name"))
	obj.TripCode = util.EscapeString(ctx.FormValue("tripcode"))
	obj.Name = util.EscapeString(ctx.FormValue("subject"))
	obj.Content = util.EscapeString(ctx.FormValue("comment"))
	obj.Sensitive = (ctx.FormValue("sensitive") != "")

	obj = ParseOptions(ctx, obj)

	var originalPost activitypub.ObjectBase
	originalPost.Id = util.EscapeString(ctx.FormValue("inReplyTo"))

	obj.InReplyTo = append(obj.InReplyTo, originalPost)

	var activity activitypub.Activity

	if !util.IsInStringArray(activity.To, originalPost.Id) {
		activity.To = append(activity.To, originalPost.Id)
	}

	if originalPost.Id != "" {
		if res, err := activitypub.IsActivityLocal(activity); err == nil && !res {
			actor, err := webfinger.FingerActor(originalPost.Id)
			if err != nil {
				return obj, err
			}

			if !util.IsInStringArray(obj.To, actor.Id) {
				obj.To = append(obj.To, actor.Id)
			}
		} else if err != nil {
			return obj, err
		}
	}

	replyingTo, err := ParseCommentForReplies(ctx.FormValue("comment"), originalPost.Id)
	if err != nil {
		return obj, err
	}

	for _, e := range replyingTo {
		has := false

		for _, f := range obj.InReplyTo {
			if e.Id == f.Id {
				has = true
				break
			}
		}

		if !has {
			obj.InReplyTo = append(obj.InReplyTo, e)

			var activity activitypub.Activity

			activity.To = append(activity.To, e.Id)

			if res, err := activitypub.IsActivityLocal(activity); err == nil && !res {
				actor, err := webfinger.FingerActor(e.Id)
				if err != nil {
					return obj, err
				}

				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			} else if err != nil {
				return obj, err
			}
		}
	}

	return obj, nil
}

func ParseOptions(ctx *fiber.Ctx, obj activitypub.ObjectBase) activitypub.ObjectBase {
	options := util.EscapeString(ctx.FormValue("options"))
	if options != "" {
		option := strings.Split(options, ";")
		email := regexp.MustCompile(".+@.+\\..+")
		wallet := regexp.MustCompile("wallet:.+")
		delete := regexp.MustCompile("delete:.+")
		for _, e := range option {
			if e == "noko" {
				obj.Option = append(obj.Option, "noko")
			} else if e == "sage" {
				obj.Option = append(obj.Option, "sage")
			} else if e == "nokosage" {
				obj.Option = append(obj.Option, "nokosage")
			} else if email.MatchString(e) {
				obj.Option = append(obj.Option, "email:"+e)
			} else if wallet.MatchString(e) {
				obj.Option = append(obj.Option, "wallet")
				var wallet activitypub.CryptoCur
				value := strings.Split(e, ":")
				wallet.Type = value[0]
				wallet.Address = value[1]
				obj.Wallet = append(obj.Wallet, wallet)
			} else if delete.MatchString(e) {
				obj.Option = append(obj.Option, e)
			}
		}
	}

	return obj
}

func CheckCaptcha(captcha string) (bool, error) {
	parts := strings.Split(captcha, ":")

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[1], " ") == "" {
		return false, nil
	}

	path := "public/" + parts[0] + ".png"
	code, err := db.GetCaptchaCodeDB(path)
	if err != nil {
		return false, err
	}

	if code != "" {
		err = db.DeleteCaptchaCodeDB(path)
		if err != nil {
			return false, err
		}

		err = db.CreateNewCaptcha()
		if err != nil {
			return false, err
		}

	}

	return code == strings.ToUpper(parts[1]), nil
}

func ParseInboxRequest(ctx *fiber.Ctx) error {
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
					col, err := activitypub.GetCollectionFromID(activity.Object.Id)
					if err != nil {
						return err
					}

					if len(col.OrderedItems) < 1 {
						break
					}

					if _, err := activitypub.WriteObjectToCache(*activity.Object); err != nil {
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
				if activity.Object.Replies != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						if err := activitypub.TombstoneObject(k.Id); err != nil {
							return err
						}
					}
				}

				if err := activitypub.TombstoneObject(activity.Object.Id); err != nil {
					return err
				}
				if err := db.UnArchiveLast(actor.Id); err != nil {
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
				autoSub, err := activitypub.GetActorAutoSubscribeDB(response.Actor.Id)
				if err != nil {
					return err
				}

				following, err := activitypub.GetActorFollowingDB(response.Actor.Id)
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

func MakeActivityFollowingReq(w http.ResponseWriter, r *http.Request, activity activitypub.Activity) (bool, error) {
	actor, err := webfinger.GetActor(activity.Object.Id)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", actor.Inbox, nil)
	if err != nil {
		return false, err
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respActivity activitypub.Activity

	err = json.Unmarshal(body, &respActivity)
	return respActivity.Type == "Accept", err
}

func IsMediaBanned(f multipart.File) (bool, error) {
	f.Seek(0, 0)

	fileBytes := make([]byte, 2048)

	_, err := f.Read(fileBytes)
	if err != nil {
		return true, err
	}

	hash := util.HashBytes(fileBytes)

	//	f.Seek(0, 0)
	return db.IsHashBanned(hash)
}

func SendToFollowers(actor string, activity activitypub.Activity) error {
	nActor, err := activitypub.GetActorFromDB(actor)
	if err != nil {
		return err
	}

	activity.Actor = &nActor

	followers, err := activitypub.GetActorFollowDB(actor)
	if err != nil {
		return err
	}

	var to []string

	for _, e := range followers {
		for _, k := range activity.To {
			if e.Id != k {
				to = append(to, e.Id)
			}
		}
	}

	activity.To = to

	if len(activity.Object.InReplyTo) > 0 {
		err = db.MakeActivityRequest(activity)
	}

	return err
}
