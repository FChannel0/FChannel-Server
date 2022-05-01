package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
)

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
