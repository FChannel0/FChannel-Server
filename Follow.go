package main

import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"

func GetActorFollowing(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, _ = GetActorFollowTotal(db, id)
	following.Items = GetActorFollowingDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetActorFollowers(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection
	
	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	_, following.TotalItems = GetActorFollowTotal(db, id)
	following.Items = GetActorFollowDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
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
	accept.Actor = activity.Object.Actor
	var nObj ObjectBase
	var nActor Actor	
	accept.Object = &nObj
	accept.Object.Actor = &nActor	
	accept.Object.Actor = activity.Actor
	var nNested NestedObjectBase
	var mActor Actor
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = &mActor
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"	
	accept.To = append(accept.To, activity.Object.Actor.Id)

	return accept
}

func RejectFollow(activity Activity) Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Reject"
	var nObj ObjectBase
	var nActor Actor	
	accept.Object = &nObj
	accept.Object.Actor = &nActor		
	accept.Actor = activity.Object.Actor
	accept.Object.Actor = activity.Actor
	var nNested NestedObjectBase
	var mActor Actor	
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = &mActor	
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Actor.Id)

	return accept	
}

func SetActorFollowerDB(db *sql.DB, activity Activity) Activity {
	var query string
	alreadyFollow := false
	followers := GetActorFollowDB(db, activity.Actor.Id)
	
	for _, e := range followers {
		if e.Id == activity.Object.Actor.Id {
			alreadyFollow = true
		}
	}
	if alreadyFollow {
		query = `delete from follower where id=$1 and follower=$2`
		activity.Summary = activity.Object.Actor.Id + " Unfollow " + activity.Actor.Id
	} else {
		query = `insert into follower (id, follower) values ($1, $2)`
		activity.Summary = activity.Object.Actor.Id + " Follow " + activity.Actor.Id
	}

	_, err := db.Exec(query, activity.Actor.Id, activity.Object.Actor.Id)

	if CheckError(err, "error with follower db insert/delete") != nil {
		activity.Type = "Reject"
		return activity
	}	

	activity.Type = "Accept"	
	return activity
}

func SetActorFollowingDB(db *sql.DB, activity Activity) Activity {
	var query string
	alreadyFollow := false
	following := GetActorFollowingDB(db, activity.Object.Actor.Id)


	for _, e := range following {
		if e.Id == activity.Actor.Id {
			alreadyFollow = true
		}
	}
	
	if alreadyFollow {
		query = `delete from following where id=$1 and following=$2`
		activity.Summary = activity.Object.Actor.Id + " Unfollowing " + activity.Actor.Id
	} else {		
		query = `insert into following (id, following) values ($1, $2)`
		activity.Summary = activity.Object.Actor.Id + " Following " + activity.Actor.Id
	}
	
	_, err := db.Exec(query, activity.Object.Actor.Id, activity.Actor.Id)

	if CheckError(err, "error with following db insert/delete") != nil {
		activity.Type = "Reject"
		return activity
	}
	
	activity.Type = "Accept"
	return	activity
}
