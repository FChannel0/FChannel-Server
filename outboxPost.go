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
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func ParseOutboxRequest(w http.ResponseWriter, r *http.Request) error {
	//var activity activitypub.Activity

	actor, err := db.GetActorFromPath(r.URL.Path, "/")
	if err != nil {
		return err
	}

	contentType := GetContentType(r.Header.Get("content-type"))

	defer r.Body.Close()

	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		r.ParseMultipartForm(5 << 20)

		hasCaptcha, err := db.BoardHasAuthType(actor.Name, "captcha")
		if err != nil {
			return err
		}

		valid, err := CheckCaptcha(r.FormValue("captcha"))
		if err == nil && hasCaptcha && valid {
			f, header, _ := r.FormFile("file")

			if header != nil {
				defer f.Close()
				if header.Size > (7 << 20) {
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					_, err := w.Write([]byte("7MB max file size"))
					return err
				} else if res, err := IsMediaBanned(f); err == nil && res {
					fmt.Println("media banned")
					http.Redirect(w, r, config.Domain, http.StatusSeeOther)
					return nil
				} else if err != nil {
					return err
				}

				contentType, _ := GetFileContentType(f)

				if !SupportedMIMEType(contentType) {
					w.WriteHeader(http.StatusNotAcceptable)
					_, err := w.Write([]byte("file type not supported"))
					return err
				}
			}

			var nObj = CreateObject("Note")
			nObj, err := ObjectFromForm(r, nObj)
			if err != nil {
				return err
			}

			nObj.Actor = config.Domain + "/" + actor.Name

			nObj, err = db.WriteObjectToDB(nObj)
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

			w.WriteHeader(http.StatusOK)
			_, err = w.Write([]byte(id))
			return err
		}

		w.WriteHeader(http.StatusForbidden)
		_, err = w.Write([]byte("captcha could not auth"))
		return err
	} else {
		activity, err := GetActivityFromJson(r)
		if err != nil {
			return err
		}

		if res, err := IsActivityLocal(activity); err == nil && res {
			if res := db.VerifyHeaderSignature(r, *activity.Actor); err == nil && !res {
				w.WriteHeader(http.StatusBadRequest)
				_, err = w.Write([]byte(""))
				return err
			}

			switch activity.Type {
			case "Create":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
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

				db.FollowingBoards, err = db.GetActorFollowingDB(config.Domain)
				if err != nil {
					return err
				}

				db.Boards, err = db.GetBoardCollection()
				if err != nil {
					return err
				}
				break

			case "Delete":
				fmt.Println("This is a delete")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))
				break

			case "Note":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))
				break

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive

				actor, err := db.CreateNewBoardDB(*CreateNewActor(name, prefname, summary, authReq, restricted))
				if err != nil {
					return err
				}

				if actor.Id != "" {
					var board []activitypub.ObjectBase
					var item activitypub.ObjectBase
					var removed bool = false

					item.Id = actor.Id
					for _, e := range db.FollowingBoards {
						if e.Id != item.Id {
							board = append(board, e)
						} else {
							removed = true
						}
					}

					if !removed {
						board = append(board, item)
					}

					db.FollowingBoards = board
					db.Boards, err = db.GetBoardCollection()
					return err
				}

				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				break

			default:
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))
			}
		} else if err != nil {
			return err
		} else {
			fmt.Println("is NOT activity")
			w.WriteHeader(http.StatusBadRequest)
			_, err = w.Write([]byte("could not process activity"))
			return err
		}
	}

	return nil
}

func ObjectFromJson(r *http.Request, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	body, _ := ioutil.ReadAll(r.Body)

	var respActivity activitypub.ActivityRaw

	err := json.Unmarshal(body, &respActivity)
	if err != nil {
		return obj, err
	}

	res, err := HasContextFromJson(respActivity.AtContextRaw.Context)

	if err == nil && res {
		var jObj activitypub.ObjectBase
		jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
		if err != nil {
			return obj, err
		}

		jObj.To, err = GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return obj, err
		}

		jObj.Cc, err = GetToFromJson(respActivity.CcRaw)
	}

	return obj, err
}

func GetObjectFromJson(obj []byte) (activitypub.ObjectBase, error) {
	var generic interface{}
	var nObj activitypub.ObjectBase

	if err := json.Unmarshal(obj, &generic); err != nil {
		return activitypub.ObjectBase{}, err
	}

	if generic != nil {
		switch generic.(type) {
		case []interface{}:
			var lObj activitypub.ObjectBase
			var arrContext activitypub.ObjectArray

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			if len(arrContext.Object) > 0 {
				lObj = arrContext.Object[0]
			}
			nObj = lObj
			break

		case map[string]interface{}:
			var arrContext activitypub.Object

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			nObj = *arrContext.Object
			break

		case string:
			var lObj activitypub.ObjectBase
			var arrContext activitypub.ObjectString

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			lObj.Id = arrContext.Object
			nObj = lObj
			break
		}
	}

	return nObj, nil
}

func GetActorFromJson(actor []byte) (activitypub.Actor, error) {
	var generic interface{}
	var nActor activitypub.Actor
	err := json.Unmarshal(actor, &generic)
	if err != nil {
		return nActor, err
	}

	if generic != nil {
		switch generic.(type) {
		case map[string]interface{}:
			err = json.Unmarshal(actor, &nActor)
			break

		case string:
			var str string
			err = json.Unmarshal(actor, &str)
			nActor.Id = str
			break
		}

		return nActor, err
	}

	return nActor, nil
}

func GetToFromJson(to []byte) ([]string, error) {
	var generic interface{}

	err := json.Unmarshal(to, &generic)
	if err != nil {
		return nil, err
	}

	if generic != nil {
		var nStr []string
		switch generic.(type) {
		case []interface{}:
			err = json.Unmarshal(to, &nStr)
			break
		case string:
			var str string
			err = json.Unmarshal(to, &str)
			nStr = append(nStr, str)
			break
		}
		return nStr, err
	}

	return nil, nil
}

func HasContextFromJson(context []byte) (bool, error) {
	var generic interface{}

	err := json.Unmarshal(context, &generic)
	if err != nil {
		return false, err
	}

	hasContext := false

	switch generic.(type) {
	case []interface{}:
		var arrContext activitypub.AtContextArray
		err = json.Unmarshal(context, &arrContext.Context)
		if len(arrContext.Context) > 0 {
			if arrContext.Context[0] == "https://www.w3.org/ns/activitystreams" {
				hasContext = true
			}
		}
		break

	case string:
		var arrContext activitypub.AtContextString
		err = json.Unmarshal(context, &arrContext.Context)
		if arrContext.Context == "https://www.w3.org/ns/activitystreams" {
			hasContext = true
		}
		break
	}

	return hasContext, err
}

func ObjectFromForm(r *http.Request, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	file, header, _ := r.FormFile("file")
	var err error

	if file != nil {
		defer file.Close()

		var tempFile = new(os.File)
		obj.Attachment, tempFile, err = CreateAttachmentObject(file, header)
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

		obj.Preview = CreatePreviewObject(obj.Attachment[0])
	}

	obj.AttributedTo = util.EscapeString(r.FormValue("name"))
	obj.TripCode = util.EscapeString(r.FormValue("tripcode"))
	obj.Name = util.EscapeString(r.FormValue("subject"))
	obj.Content = util.EscapeString(r.FormValue("comment"))
	obj.Sensitive = (r.FormValue("sensitive") != "")

	obj = ParseOptions(r, obj)

	var originalPost activitypub.ObjectBase
	originalPost.Id = util.EscapeString(r.FormValue("inReplyTo"))

	obj.InReplyTo = append(obj.InReplyTo, originalPost)

	var activity activitypub.Activity

	if !util.IsInStringArray(activity.To, originalPost.Id) {
		activity.To = append(activity.To, originalPost.Id)
	}

	if originalPost.Id != "" {
		if res, err := IsActivityLocal(activity); err == nil && !res {
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

	replyingTo, err := ParseCommentForReplies(r.FormValue("comment"), originalPost.Id)
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

			if res, err := IsActivityLocal(activity); err == nil && !res {
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

func ParseOptions(r *http.Request, obj activitypub.ObjectBase) activitypub.ObjectBase {
	options := util.EscapeString(r.FormValue("options"))
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

func GetActivityFromJson(r *http.Request) (activitypub.Activity, error) {
	body, _ := ioutil.ReadAll(r.Body)

	var respActivity activitypub.ActivityRaw
	var nActivity activitypub.Activity
	var nType string

	if err := json.Unmarshal(body, &respActivity); err != nil {
		return nActivity, err
	}

	if res, err := HasContextFromJson(respActivity.AtContextRaw.Context); err == nil && res {
		var jObj activitypub.ObjectBase

		if respActivity.Type == "Note" {
			jObj, err = GetObjectFromJson(body)
			if err != nil {
				return nActivity, err
			}

			nType = "Create"
		} else {
			jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
			if err != nil {
				return nActivity, err
			}

			nType = respActivity.Type
		}

		actor, err := GetActorFromJson(respActivity.ActorRaw)
		if err != nil {
			return nActivity, err
		}

		to, err := GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return nActivity, err
		}

		cc, err := GetToFromJson(respActivity.CcRaw)
		if err != nil {
			return nActivity, err
		}

		nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		nActivity.Type = nType
		nActivity.Actor = &actor
		nActivity.Published = respActivity.Published
		nActivity.Auth = respActivity.Auth

		if len(to) > 0 {
			nActivity.To = to
		}

		if len(cc) > 0 {
			nActivity.Cc = cc
		}

		nActivity.Name = respActivity.Name
		nActivity.Object = &jObj
	} else if err != nil {
		return nActivity, err
	}

	return nActivity, nil
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

func ParseInboxRequest(w http.ResponseWriter, r *http.Request) error {
	activity, err := GetActivityFromJson(r)
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

	if !db.VerifyHeaderSignature(r, *activity.Actor) {
		response := db.RejectActivity(activity)

		return db.MakeActivityRequest(response)
	}

	switch activity.Type {
	case "Create":

		for _, e := range activity.To {
			if res, err := db.IsActorLocal(e); err == nil && res {
				if res, err := db.IsActorLocal(activity.Actor.Id); err == nil && res {
					col, err := GetCollectionFromID(activity.Object.Id)
					if err != nil {
						return err
					}

					if len(col.OrderedItems) < 1 {
						break
					}

					if _, err := db.WriteObjectToCache(*activity.Object); err != nil {
						return err
					}

					actor, err := db.GetActorFromDB(e)
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
			actor, err := db.GetActorFromDB(e)
			if err != nil {
				return err
			}

			if actor.Id != "" && actor.Id != config.Domain {
				if activity.Object.Replies != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						if err := db.TombstoneObject(k.Id); err != nil {
							return err
						}
					}
				}

				if err := db.TombstoneObject(activity.Object.Id); err != nil {
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
			if res, err := db.GetActorFromDB(e); err == nil && res.Id != "" {
				response := db.AcceptFollow(activity)
				response, err := db.SetActorFollowerDB(response)
				if err != nil {
					return err
				}

				if err := db.MakeActivityRequest(response); err != nil {
					return err
				}

				alreadyFollow := false
				alreadyFollowing := false
				autoSub, err := db.GetActorAutoSubscribeDB(response.Actor.Id)
				if err != nil {
					return err
				}

				following, err := db.GetActorFollowingDB(response.Actor.Id)
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
				response := db.RejectActivity(activity)

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
	nActor, err := db.GetActorFromDB(actor)
	if err != nil {
		return err
	}

	activity.Actor = &nActor

	followers, err := db.GetActorFollowDB(actor)
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
