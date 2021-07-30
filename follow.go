package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	_ "github.com/lib/pq"
)

func GetActorFollowing(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, _ = GetActorFollowTotal(db, id)
	following.Items = GetActorFollowingDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}

func GetActorFollowers(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	_, following.TotalItems = GetActorFollowTotal(db, id)
	following.Items = GetActorFollowDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}


func GetActorFollowingDB(db *sql.DB, id string) []ObjectBase {
	var followingCollection []ObjectBase
	query := `select following from following where id=$1`

	rows, err := db.Query(query, id)

	CheckError(err, "error with following db query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		err := rows.Scan(&obj.Id)

		CheckError(err, "error with following db scan")

		followingCollection = append(followingCollection, obj)
	}

	return followingCollection
}

func GetActorFollowDB(db *sql.DB, id string) []ObjectBase {
	var followerCollection []ObjectBase

	query := `select follower from follower where id=$1`

	rows, err := db.Query(query, id)

	CheckError(err, "error with follower db query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		err := rows.Scan(&obj.Id)

		CheckError(err, "error with followers db scan")

		followerCollection = append(followerCollection, obj)
	}

	return followerCollection
}

func GetActorFollowTotal(db *sql.DB, id string) (int, int) {
	var following int
	var followers int

	query := `select count(following) from following where id=$1`

	rows, err := db.Query(query, id)

	CheckError(err, "error with following total db query")

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&following)

		CheckError(err, "error with following total db scan")
	}

	query = `select count(follower) from follower where id=$1`

	rows, err = db.Query(query, id)

	CheckError(err, "error with followers total db query")

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&followers)

		CheckError(err, "error with followers total db scan")
	}

	return following, followers
}

func AcceptFollow(activity Activity) Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Accept"
	var nActor Actor
	accept.Actor = &nActor
	accept.Actor.Id = activity.Object.Actor
	var nObj ObjectBase
	accept.Object = &nObj
	accept.Object.Actor = activity.Actor.Id
	var nNested NestedObjectBase
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Object.Actor)

	return accept
}

func RejectActivity(activity Activity) Activity {
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

func IsAlreadyFollowing(db *sql.DB, actor string, follow string) bool {
	followers := GetActorFollowingDB(db, actor)

	for _, e := range followers {
		if e.Id == follow {
			return true
		}
	}

	return false;
}

func IsAlreadyFollower(db *sql.DB, actor string, follow string) bool {
	followers := GetActorFollowDB(db, actor)

	for _, e := range followers {
		if e.Id == follow {
			return true
		}
	}

	return false;
}

func SetActorFollowerDB(db *sql.DB, activity Activity) Activity {
	var query string
	alreadyFollow := IsAlreadyFollower(db, activity.Actor.Id, activity.Object.Actor)

	activity.Type = "Reject"
	if activity.Actor.Id == activity.Object.Actor {
		return activity
	}

	if alreadyFollow {
		query = `delete from follower where id=$1 and follower=$2`
		activity.Summary = activity.Object.Actor + " Unfollow " + activity.Actor.Id

		_, err := db.Exec(query, activity.Actor.Id, activity.Object.Actor)

		if CheckError(err, "error with follower db delete") != nil {
			activity.Type = "Reject"
			return activity
		}

		activity.Type = "Accept"
		return activity
	} else {
			query = `insert into follower (id, follower) values ($1, $2)`
		activity.Summary = activity.Object.Actor + " Follow " + activity.Actor.Id

		_, err := db.Exec(query, activity.Actor.Id, activity.Object.Actor)

		if CheckError(err, "error with follower db insert") != nil {
			activity.Type = "Reject"
			return activity
		}

		activity.Type = "Accept"
		return activity
	}


	return activity
}

func SetActorFollowingDB(db *sql.DB, activity Activity) Activity {
	var query string
	alreadyFollowing := false
	alreadyFollower := false
	following := GetActorFollowingDB(db, activity.Object.Actor)

	actor := FingerActor(activity.Actor.Id)

	remoteActorFollowerCol := GetCollectionFromReq(actor.Followers)

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
		return activity
	}

	if alreadyFollowing && alreadyFollower {
		query = `delete from following where id=$1 and following=$2`
		activity.Summary = activity.Object.Actor + " Unfollowing " + activity.Actor.Id
		if !IsActorLocal(db, activity.Actor.Id) {
			go DeleteActorCache(db, activity.Actor.Id)
		}
		_, err := db.Exec(query, activity.Object.Actor, activity.Actor.Id)

		if CheckError(err, "error with following db delete") != nil {
			activity.Type = "Reject"
			return activity
		}

		activity.Type = "Accept"
		return activity
	}

	if !alreadyFollowing && !alreadyFollower {

		query = `insert into following (id, following) values ($1, $2)`
		activity.Summary = activity.Object.Actor + " Following " + activity.Actor.Id
		if !IsActorLocal(db, activity.Actor.Id) {
			go WriteActorToCache(db, activity.Actor.Id)
		}
		_, err := db.Exec(query, activity.Object.Actor, activity.Actor.Id)

		if CheckError(err, "error with following db insert") != nil {
			activity.Type = "Reject"
			return activity
		}

		activity.Type = "Accept"
		return activity
	}


	return	activity
}

func AutoFollow(db *sql.DB, actor string) {
	following := GetActorFollowingDB(db, actor)
	follower := GetActorFollowDB(db, actor)

	isFollowing := false

	for _, e := range follower {
		for _, k := range following {
			if e.Id == k.Id {
				isFollowing = true
			}
		}

		if !isFollowing && e.Id != Domain && e.Id != actor {
			followActivity := MakeFollowActivity(db, actor, e.Id)

			nActor := FingerActor(e.Id)

			if nActor.Id != "" {
				MakeActivityRequestOutbox(db, followActivity)
			}
		}
	}
}

func MakeFollowActivity(db *sql.DB, actor string, follow string) Activity {
	var followActivity Activity

	followActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	followActivity.Type = "Follow"

	var obj ObjectBase
	var nactor Actor
	if actor == Domain {
		nactor = GetActorFromDB(db, actor)
	} else {
		nactor = FingerActor(actor)
	}

	followActivity.Actor = &nactor
	followActivity.Object = &obj

	followActivity.Object.Actor = follow
	followActivity.To = append(followActivity.To, follow)

	return followActivity
}
