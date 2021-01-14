package main

import "fmt"
import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"

func GetActorOutbox(w http.ResponseWriter, r *http.Request, db *sql.DB) {

	actor := GetActorFromPath(db, r.URL.Path, "/")
	var collection Collection

	collection.OrderedItems = GetObjectFromDB(db, actor).OrderedItems

	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = actor.Id

	collection.TotalItems = GetObjectPostsTotalDB(db, actor)
	collection.TotalImgs = GetObjectImgsTotalDB(db, actor)	

	enc, _ := json.MarshalIndent(collection, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}



func GetObjectsFromFollow(actor Actor) []ObjectBase {
	var followingCol Collection
	var followObj []ObjectBase
	followingCol = GetActorCollection(actor.Following)
	for _, e := range followingCol.Items {
		var followOutbox Collection
		var actor Actor
		actor = GetActor(e.Id)
		followOutbox = GetActorCollection(actor.Outbox)
		for _, e := range followOutbox.OrderedItems {
			followObj = append(followObj, e)
		}
	}
	return followObj
}

func GetCollectionFromPath(db *sql.DB, path string) Collection {

	var nColl Collection
	var result []ObjectBase

	query := fmt.Sprintf("SELECT id, name, content, type, published, attributedto, attachment, actor FROM activitystream WHERE id='%s' ORDER BY published desc;", path)

	rows, err := db.Query(query)

	CheckError(err, "error query collection path from db")
	
	defer rows.Close()

	for rows.Next(){
		var actor Actor
		var post ObjectBase
		var attachID string
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &actor.Id)
		
		CheckError(err, "error scan object into post struct from path")

		post.Actor = &actor

		post.InReplyTo = GetInReplyToDB(db, post)

		post.Replies = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

		post.Attachment = GetObjectAttachment(db, attachID)

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"	

	nColl.OrderedItems = result

	return nColl		
}

func GetObjectFromPath(db *sql.DB, path string) ObjectBase{

	var nObj ObjectBase
	var result []ObjectBase

	query := fmt.Sprintf("SELECT id, name, content, type, published, attributedto, attachment, actor FROM activitystream WHERE id='%s' ORDER BY published desc;", path)

	rows, err := db.Query(query)

	CheckError(err, "error query collection path from db")
	
	defer rows.Close()

	for rows.Next(){
		var post ObjectBase
		var attachID string
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &post.Actor)
		
		CheckError(err, "error scan object into post struct from path")

		post.Replies = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

		post.Attachment = GetObjectAttachment(db, attachID)

		result = append(result, post)
	}

	nObj = result[0]

	return nObj		
}
