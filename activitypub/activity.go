package activitypub

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

func AcceptActivity(header string) bool {
	accept := false
	if strings.Contains(header, ";") {
		split := strings.Split(header, ";")
		accept = accept || activityRegexp.MatchString(split[0])
		accept = accept || strings.Contains(split[len(split)-1], "profile=\"https://www.w3.org/ns/activitystreams\"")
	} else {
		accept = accept || activityRegexp.MatchString(header)
	}
	return accept
}

func (activity Activity) IsLocal() (bool, error) {

	for _, e := range activity.To {
		if res, _ := GetActorFromDB(e); res.Id != "" {
			return true, nil
		}
	}

	for _, e := range activity.Cc {
		if res, _ := GetActorFromDB(e); res.Id != "" {
			return true, nil
		}
	}

	if activity.Actor != nil {
		if res, _ := GetActorFromDB(activity.Actor.Id); res.Id != "" {
			return true, nil
		}
	}

	return false, nil
}

func (activity Activity) Process() error {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if res, err := GetActorFromDB(e); res.Id != "" {
				fmt.Println("actor is in the database")
			} else if err != nil {
				return util.MakeError(err, "Process")
			} else {
				fmt.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {
		// TODO: okay?
		return errors.New("not implemented")
	} else if activityType == "Delete" {
		return errors.New("not implemented")
	}

	return nil
}

func (activity Activity) Reject() Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Reject"
	var nObj ObjectBase
	accept.Object = &nObj
	var nActor Actor
	accept.Actor = &nActor
	accept.Actor.Id = activity.Object.Actor
	accept.Object.Actor = activity.Actor.Id
	var nNested NestedObjectBase
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Actor.Id)

	return accept
}

func (activity Activity) Report(reason string) (bool, error) {
	if isLocal, err := activity.Object.IsLocal(); !isLocal || err != nil {
		return false, util.MakeError(err, "Report")
	}

	activityCol, err := activity.Object.GetCollection()
	if err != nil {
		return false, util.MakeError(err, "Report")
	}

	query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`
	if _, err = config.DB.Exec(query, activity.Object.Object.Id, 1, activityCol.Actor.Id, reason); err != nil {
		return false, util.MakeError(err, "Report")
	}

	return true, nil
}

func (activity Activity) SetFollower() (Activity, error) {
	var query string

	alreadyFollow, err := activity.Actor.IsAlreadyFollower(activity.Object.Actor)

	if err != nil {
		return activity, util.MakeError(err, "SetFollower")
	}

	activity.Type = "Reject"
	if activity.Actor.Id == activity.Object.Actor {
		return activity, nil
	}

	if alreadyFollow {
		query = `delete from follower where id=$1 and follower=$2`
		activity.Summary = activity.Object.Actor + " Unfollow " + activity.Actor.Id

		if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
			return activity, util.MakeError(err, "SetFollower")
		}

		activity.Type = "Accept"
		return activity, util.MakeError(err, "SetFollower")
	}

	query = `insert into follower (id, follower) values ($1, $2)`
	activity.Summary = activity.Object.Actor + " Follow " + activity.Actor.Id

	if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
		return activity, util.MakeError(err, "SetFollower")
	}

	activity.Type = "Accept"
	return activity, nil
}
