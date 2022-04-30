package db

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func AcceptFollow(activity activitypub.Activity) activitypub.Activity {
	var accept activitypub.Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Accept"
	var nActor activitypub.Actor
	accept.Actor = &nActor
	accept.Actor.Id = activity.Object.Actor
	var nObj activitypub.ObjectBase
	accept.Object = &nObj
	accept.Object.Actor = activity.Actor.Id
	var nNested activitypub.NestedObjectBase
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Object.Actor)

	return accept
}

func SetActorFollowingDB(activity activitypub.Activity) (activitypub.Activity, error) {
	var query string
	alreadyFollowing := false
	alreadyFollower := false
	following, err := activitypub.GetActorFollowingDB(activity.Object.Actor)
	if err != nil {
		return activity, err
	}

	actor, err := webfinger.FingerActor(activity.Actor.Id)
	if err != nil {
		return activity, err
	}

	remoteActorFollowerCol, err := webfinger.GetCollectionFromReq(actor.Followers)
	if err != nil {
		return activity, err
	}

	for _, e := range following {
		if e.Id == activity.Actor.Id {
			alreadyFollowing = true
		}
	}

	for _, e := range remoteActorFollowerCol.Items {
		if e.Id == activity.Object.Actor {
			alreadyFollower = true
		}
	}

	activity.Type = "Reject"

	if activity.Actor.Id == activity.Object.Actor {
		return activity, nil
	}

	if alreadyFollowing && alreadyFollower {
		query = `delete from following where id=$1 and following=$2`
		activity.Summary = activity.Object.Actor + " Unfollowing " + activity.Actor.Id
		if res, err := activitypub.IsActorLocal(activity.Actor.Id); err == nil && !res {
			go activitypub.DeleteActorCache(activity.Actor.Id)
		} else {
			return activity, err
		}

		if _, err := config.DB.Exec(query, activity.Object.Actor, activity.Actor.Id); err != nil {
			return activity, err
		}

		activity.Type = "Accept"
		return activity, nil
	}

	if !alreadyFollowing && !alreadyFollower {

		query = `insert into following (id, following) values ($1, $2)`
		activity.Summary = activity.Object.Actor + " Following " + activity.Actor.Id
		if res, err := activitypub.IsActorLocal(activity.Actor.Id); err == nil && !res {
			go WriteActorToCache(activity.Actor.Id)
		}
		if _, err := config.DB.Exec(query, activity.Object.Actor, activity.Actor.Id); err != nil {
			return activity, err
		}

		activity.Type = "Accept"
		return activity, nil
	}

	return activity, nil
}

func AutoFollow(actor string) error {
	following, err := activitypub.GetActorFollowingDB(actor)
	if err != nil {
		return err
	}

	follower, err := activitypub.GetActorFollowDB(actor)
	if err != nil {
		return err
	}

	isFollowing := false

	for _, e := range follower {
		for _, k := range following {
			if e.Id == k.Id {
				isFollowing = true
			}
		}

		if !isFollowing && e.Id != config.Domain && e.Id != actor {
			followActivity, err := MakeFollowActivity(actor, e.Id)
			if err != nil {
				return err
			}

			nActor, err := webfinger.FingerActor(e.Id)
			if err != nil {
				return err
			}

			if nActor.Id != "" {
				MakeActivityRequestOutbox(followActivity)
			}
		}
	}

	return nil
}

func MakeFollowActivity(actor string, follow string) (activitypub.Activity, error) {
	var followActivity activitypub.Activity
	var err error

	followActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	followActivity.Type = "Follow"

	var obj activitypub.ObjectBase
	var nactor activitypub.Actor
	if actor == config.Domain {
		nactor, err = activitypub.GetActorFromDB(actor)
	} else {
		nactor, err = webfinger.FingerActor(actor)
	}

	if err != nil {
		return followActivity, err
	}

	followActivity.Actor = &nactor
	followActivity.Object = &obj

	followActivity.Object.Actor = follow
	followActivity.To = append(followActivity.To, follow)

	return followActivity, nil
}

func MakeActivityRequestOutbox(activity activitypub.Activity) error {
	j, _ := json.Marshal(activity)

	if activity.Actor.Outbox == "" {
		// TODO: good enough?
		return errors.New("invalid outbox")
	}

	req, err := http.NewRequest("POST", activity.Actor.Outbox, bytes.NewBuffer(j))
	if err != nil {
		return err
	}

	re := regexp.MustCompile("https?://(www.)?")

	var instance string
	if activity.Actor.Id == config.Domain {
		instance = re.ReplaceAllString(config.Domain, "")
	} else {
		_, instance = activitypub.GetActorInstance(activity.Actor.Id)
	}

	date := time.Now().UTC().Format(time.RFC1123)
	path := strings.Replace(activity.Actor.Outbox, instance, "", 1)

	path = re.ReplaceAllString(path, "")

	sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
	encSig, err := activitypub.ActivitySign(*activity.Actor, sig)
	if err != nil {
		return err
	}

	signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

	req.Header.Set("Content-Type", config.ActivityStreams)
	req.Header.Set("Date", date)
	req.Header.Set("Signature", signature)
	req.Host = instance

	_, err = util.RouteProxy(req)
	return err
}

func MakeActivityRequest(activity activitypub.Activity) error {
	j, _ := json.MarshalIndent(activity, "", "\t")

	for _, e := range activity.To {
		if e != activity.Actor.Id {
			actor, err := webfinger.FingerActor(e)
			if err != nil {
				return err
			}

			if actor.Id != "" {
				_, instance := activitypub.GetActorInstance(actor.Id)

				if actor.Inbox != "" {
					req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))
					if err != nil {
						return err
					}

					date := time.Now().UTC().Format(time.RFC1123)
					path := strings.Replace(actor.Inbox, instance, "", 1)

					re := regexp.MustCompile("https?://(www.)?")
					path = re.ReplaceAllString(path, "")

					sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
					encSig, err := activitypub.ActivitySign(*activity.Actor, sig)
					if err != nil {
						return err
					}

					signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

					req.Header.Set("Content-Type", config.ActivityStreams)
					req.Header.Set("Date", date)
					req.Header.Set("Signature", signature)
					req.Host = instance

					_, err = util.RouteProxy(req)
					if err != nil {
						fmt.Println("error with sending activity resp to actor " + instance)
						return err // TODO: needs further testing
					}
				}
			}
		}
	}

	return nil
}
