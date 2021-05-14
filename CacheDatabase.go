package main

import "fmt"
import "time"
import "database/sql"
import _ "github.com/lib/pq"

func WriteObjectToCache(db *sql.DB, obj ObjectBase) ObjectBase {
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			WritePreviewToCache(db, *obj.Preview)
		}
		
		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(db, obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(db, obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(db, obj)
	}

	WriteObjectReplyToDB(db, obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteObjectToCache(db, e)
		}
	}

	return obj
}

func WriteObjectUpdatesToCache(db *sql.DB, obj ObjectBase) {
	query := `update cacheactivitystream set updated=$1 where id=$2`
	
	_, e := db.Exec(query, time.Now().Format(time.RFC3339), obj.Id)
	
	if e != nil{
		fmt.Println("error inserting updating inreplyto")
		panic(e)			
	}		
}

func WriteActivitytoCache(db *sql.DB, obj ObjectBase) {

	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting  obj id from cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}		

	query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id, obj.TripCode)	
	
	if e != nil{
		fmt.Println("error inserting new activity cache")
		panic(e)			
	}	
}

func WriteActivitytoCacheWithAttachment(db *sql.DB, obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) {
	
	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting activity with attachment obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}		

	query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Published, obj.AttributedTo, obj.Actor.Id, obj.TripCode)	
	
	if e != nil{
		fmt.Println("error inserting new activity with attachment cache")
		panic(e)			
	}	
}

func WriteAttachmentToCache(db *sql.DB, obj ObjectBase) {

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting attachment obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}
	
	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Published, obj.AttributedTo, obj.MediaType, obj.Size)	
	
	if e != nil{
		fmt.Println("error inserting new attachment cache")
		panic(e)			
	}
}

func WritePreviewToCache(db *sql.DB, obj NestedObjectBase) {

	query := `select id from cacheactivitystream where id=$1`

	rows, err := db.Query(query, obj.Id)

	CheckError(err, "error selecting preview obj id cache")

	var id string 
	defer rows.Close()
	rows.Next()
	rows.Scan(&id)

	if id != "" {
		return
	}	
	
	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	_, e := db.Exec(query, obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Published, obj.AttributedTo, obj.MediaType, obj.Size)
	
	if e != nil{
		fmt.Println("error inserting new preview cache")
		panic(e)			
	}
}

func WriteObjectReplyToCache(db *sql.DB, obj ObjectBase) {
	
	for i, e := range obj.InReplyTo {
		if(i == 0 || IsReplyInThread(db, obj.InReplyTo[0].Id, e.Id)){

			query := `select id from replies where id=$1`

			rows, err := db.Query(query, obj.Id)

			CheckError(err, "error selecting obj id cache reply")

			var id string 
			defer rows.Close()
			rows.Next()
			rows.Scan(&id)

			if id != "" {
				return
			}
			
			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			_, err = db.Exec(query, obj.Id, e.Id)			
			
			if err != nil{
				fmt.Println("error inserting replies cache")
				panic(err)			
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `insert into cachereplies (id, inreplyto) values ($1, $2)`

		_, err := db.Exec(query, obj.Id, "")			
		
		if err != nil{
			fmt.Println("error inserting replies cache")
			panic(err)			
		}
	}	
}

func WriteObjectReplyCache(db *sql.DB, obj ObjectBase) {

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {

			query := `select inreplyto from cachereplies where id=$1`

			rows, err := db.Query(query, obj.Id)

			CheckError(err, "error selecting obj id cache reply")

			var inreplyto string 		
			defer rows.Close()
			rows.Next()
			rows.Scan(&inreplyto)

			if inreplyto != "" {
				return
			}
			
			query = `insert into cachereplies (id, inreplyto) values ($1, $2)`

			_, err = db.Exec(query, e.Id, obj.Id)			
			
			if err != nil{
				fmt.Println("error inserting replies cache")
				panic(err)			
			}

			if !IsObjectLocal(db, e.Id) {
				WriteObjectToCache(db, e)
			}

		}
		return 
	}
}

func WriteActorToCache(db *sql.DB, actorID string) {
	actor := GetActor(actorID)
	collection := GetActorCollection(actor.Outbox)

	for _, e := range collection.OrderedItems {
		WriteObjectToCache(db, e)
	}
}

func DeleteObjectFromCache(db *sql.DB, id string) {
	query := `select attachment, preview from cacheactivitystream where id=$1 `

	rows, err := db.Query(query, id)
	CheckError(err, "could not select cache activitystream")

	var attachment string
	var preview string
	
	defer rows.Close()	
	rows.Next()
	rows.Scan(&attachment, &preview)

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, attachment)
	CheckError(err, "could not delete attachmet cache activitystream")

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, preview)
	CheckError(err, "could not delete preview cache activitystream")

	query = `delete from cacheactivitystream where id=$1`
	_, err = db.Exec(query, id)
	CheckError(err, "could not delete object cache activitystream")

	query = `delete from replies where id=$1`
	_, err = db.Exec(query, id)
	CheckError(err, "could not delete  cache replies activitystream")
}

func DeleteActorCache(db *sql.DB, actorID string) { 
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1)`

	rows, err := db.Query(query, actorID)

	CheckError(err, "error selecting actors activity from cache")

	defer rows.Close()

	for rows.Next() {
		var id string
		rows.Scan(&id)

		DeleteObjectFromCache(db, id)
	}
}

func TombstoneObjectFromCache(db *sql.DB, id string) {

	datetime := time.Now().Format(time.RFC3339)
	
	query := `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', updated=$1, deleted=$2 where id=$3`

	_, err := db.Exec(query, datetime, datetime, id)	

	CheckError(err, "error with tombstone cache object")

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', updated=$2, deleted=$3 where id in (select attachment from cacheactivitystream where id=$4)`

	_, err = db.Exec(query, "/public/removed.png", datetime, datetime, id)

	CheckError(err, "error with tombstone attachment cache object")

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', updated=$2, deleted=$3 where id in (select preview from cacheactivitystream where id=$4)`

	_, err = db.Exec(query, "/public/removed.png", datetime, datetime, id)

	CheckError(err, "error with tombstone preview cache object")	
	
	query = `delete from replies where id=$1`
	_, err = db.Exec(query, id)
	
	CheckError(err, "could not delete  cache replies activitystream")
}
