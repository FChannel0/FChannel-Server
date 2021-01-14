package main

import "fmt"
import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"

func GetActorFollowing(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, _ = GetActorFollowTotal(db, id)
	following.Items, _ = GetActorFollowDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetActorFollowers(w http.ResponseWriter, db *sql.DB, id string) {
	var following Collection
	
	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	_, following.TotalItems = GetActorFollowTotal(db, id)
	_, following.Items = GetActorFollowDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func SetActorFollowDB(db *sql.DB, activity Activity, actor string) Activity {
	var query string
	alreadyFollow := false
	following, follower := GetActorFollowDB(db, actor)
	
	if activity.Actor.Id == actor {
		for _, e := range following {
			if e.Id == activity.Object.Id {
				alreadyFollow = true
			}
		}
		if alreadyFollow {
			query = fmt.Sprintf("delete from following where id='%s' and following='%s'", activity.Actor.Id, activity.Object.Id)
			activity.Summary = activity.Actor.Id + " Unfollow " + activity.Object.Id
		} else {
			query = fmt.Sprintf("insert into following (id, following) values ('%s', '%s')", activity.Actor.Id, activity.Object.Id)
			activity.Summary = activity.Actor.Id + " Follow " + activity.Object.Id			
		}
	} else {
		for _, e := range follower {
			if e.Id == activity.Actor.Id {
				alreadyFollow = true
			}
		}
		if alreadyFollow {
			query = fmt.Sprintf("delete from follower where id='%s' and follower='%s'", activity.Object.Id, activity.Actor.Id)
			activity.Summary = activity.Actor.Id + " Unfollow " + activity.Object.Id			
		} else {		
			query = fmt.Sprintf("insert into follower (id, follower) values ('%s', '%s')", activity.Object.Id, activity.Actor.Id)
			activity.Summary = activity.Actor.Id + " Follow " + activity.Object.Id						
		}
	}
	
	_, err := db.Exec(query)

	CheckError(err, "error with follow db insert/delete")

	return activity
}

func GetActorFollowDB(db *sql.DB, id string) ([]ObjectBase, []ObjectBase) {
	var followingCollection []ObjectBase
	var followerCollection []ObjectBase	

	query := fmt.Sprintf("SELECT following FROM following WHERE id='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "error with following db query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase
		
		err := rows.Scan(&obj.Id)

		CheckError(err, "error with following db scan")

		followingCollection = append(followingCollection, obj)
	}

	query = fmt.Sprintf("SELECT follower FROM follower WHERE id='%s'", id)

	rows, err = db.Query(query)

	CheckError(err, "error with followers db query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase
		
		err := rows.Scan(&obj.Id)

		CheckError(err, "error with followers db scan")

		followerCollection = append(followerCollection, obj)
	}
	
	return followingCollection, followerCollection
}

func GetActorFollowTotal(db *sql.DB, id string) (int, int) {
	var following int
	var followers int

	query := fmt.Sprintf("SELECT COUNT(following) FROM following WHERE id='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "error with following total db query")

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&following)

		CheckError(err, "error with following total db scan")
	}

	query = fmt.Sprintf("SELECT COUNT(follower) FROM follower WHERE id='%s'", id)

	rows, err = db.Query(query)

	CheckError(err, "error with followers total db query")

	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&followers)

		CheckError(err, "error with followers total db scan")
	}		
	
	return following, followers
}

func AcceptFollow(activity Activity, actor Actor) Activity {
	var accept Activity
	var obj ObjectBase

	obj.Type = activity.Type
	obj.Actor = activity.Actor

	var nobj NestedObjectBase
	obj.Object = &nobj
	obj.Object.Id = activity.Object.Id

	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Accept"

	var nactor Actor
	accept.Actor = &nactor
	accept.Actor.Id = actor.Id
	accept.Object = &obj
	accept.To = append(accept.To, activity.Object.Id)

	return accept
}

func RejectFollow(activity Activity, actor Actor) Activity {
	var accept Activity
	var obj ObjectBase

	obj.Type = activity.Type
	obj.Actor = activity.Actor
	obj.Object = new(NestedObjectBase)
	obj.Object.Id = activity.Object.Id

	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Reject"
	accept.Actor = &actor
	accept.Object = &obj

	return accept
}

func SetActorFollowingDB(db *sql.DB, activity Activity) Activity{
	var query string
	alreadyFollow := false
	_, follower := GetActorFollowDB(db, activity.Object.Id)

	for _, e := range follower {
		if e.Id == activity.Object.Id {
			alreadyFollow = true
		}
	}
	
	if alreadyFollow {
		query = fmt.Sprintf("delete from follower where id='%s' and follower='%s'", activity.Object.Id, activity.Actor.Id)
		activity.Summary = activity.Actor.Id + " Unfollow " + activity.Object.Id			
	} else {		
		query = fmt.Sprintf("insert into follower (id, follower) values ('%s', '%s')", activity.Object.Id, activity.Actor.Id)
		activity.Summary = activity.Actor.Id + " Follow " + activity.Object.Id						
	}
	
	_, err := db.Exec(query)

	if err != nil {
		CheckError(err, "error with follow db insert/delete")
		activity.Type = "Reject"
		return activity
	}
	
	activity.Type = "Accept"
	return	activity
}
