package main

import "fmt"
import "database/sql"
import _ "github.com/lib/pq"
import "time"
import "os"
import "strings"
import "regexp"

func GetActorFromDB(db *sql.DB, id string) Actor {
	var nActor Actor
	
	query := fmt.Sprintf("SELECT type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary from actor where id='%s'", id)
	rows, err := db.Query(query)

	if CheckError(err, "could not get actor from db query") != nil {
		return nActor
	}

	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary)
		CheckError(err, "error with actor from db scan ")
	}

	nActor.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	return nActor	
}

func CreateNewBoardDB(db *sql.DB, actor Actor) Actor{

	query := fmt.Sprintf("INSERT INTO actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary) values ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')", actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary)

	_, err := db.Exec(query)

	if err != nil {
		fmt.Println("board exists")
	} else {
		fmt.Println("board added")
		for _, e := range actor.AuthRequirement { 
			query  = fmt.Sprintf("INSERT INTO actorauth (type, board) values ('%s', '%s')", e, actor.Name)
			_, err := db.Exec(query)
			CheckError(err, "auth exists")
		}

		var verify Verify

		verify.Identifier = actor.Id
		verify.Code  = CreateKey(50)
		verify.Type  = "admin"

		CreateVerification(db, verify)

		verify.Identifier = actor.Id
		verify.Code  = CreateKey(50)
		verify.Type  = "janitor"

		CreateVerification(db, verify)

		var nverify Verify
		nverify.Board = actor.Id
		nverify.Identifier = "admin"
		nverify.Type = "admin"
		CreateBoardMod(db, nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "janitor"
		nverify.Type = "janitor"
		CreateBoardMod(db, nverify)			

		if actor.Name != "main" {
			var nActivity Activity
			var nActor Actor
			var nObject ObjectBase

			nActor.Id = Domain
			nObject.Id = actor.Id

			nActivity.Actor = &nActor
			nActivity.Object = &nObject

			SetActorFollowDB(db, nActivity, Domain)
		}
	}

	return actor
}

func GetBoards(db *sql.DB) []Actor {

	var board []Actor
	
	query := fmt.Sprintf("select type, id, name, preferedusername, inbox, outbox, following, followers FROM actor")

	rows, err := db.Query(query)

	CheckError(err, "could not get boards from db query")

	defer rows.Close()	
	for rows.Next(){
		var actor = new(Actor)
		
		err = rows.Scan(&actor.Type, &actor.Id, &actor.Name, &actor.PreferredUsername, &actor.Inbox, &actor.Outbox, &actor.Following, &actor.Followers)
		
		if err !=nil{
			panic(err)
		}

		board = append(board, *actor)
	}

	return board
}

func writeObjectToDB(db *sql.DB, obj ObjectBase) ObjectBase {
	obj.Id = fmt.Sprintf("%s/%s", obj.Actor.Id, CreateUniqueID(db, obj.Actor.Id))
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor.Id, CreateUniqueID(db, obj.Actor.Id))
			obj.Preview.Published = time.Now().Format(time.RFC3339)
			obj.Preview.Updated = time.Now().Format(time.RFC3339)			
			obj.Preview.AttributedTo = obj.Id
			WritePreviewToDB(db, *obj.Preview)
		}
		
		for i, _ := range obj.Attachment {
			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor.Id, CreateUniqueID(db, obj.Actor.Id))			
			obj.Attachment[i].Published = time.Now().Format(time.RFC3339)
			obj.Attachment[i].Updated = time.Now().Format(time.RFC3339)
			obj.Attachment[i].AttributedTo = obj.Id
			writeAttachmentToDB(db, obj.Attachment[i])
			writeActivitytoDBWithAttachment(db, obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		writeActivitytoDB(db, obj)
	}

	writeObjectReplyToDB(db, obj)
	WriteWalletToDB(db, obj)

	return obj
}

func WriteObjectUpdatesToDB(db *sql.DB, obj ObjectBase) {
	query := fmt.Sprintf("update activitystream set updated='%s' where id='%s'", time.Now().Format(time.RFC3339), obj.Id)
	_, e := db.Exec(query)
	
	if e != nil{
		fmt.Println("error inserting updating inreplyto")
		panic(e)			
	}		
}

func WriteObjectReplyToLocalDB(db *sql.DB, id string, replyto string) {
	query := fmt.Sprintf("insert into replies (id, inreplyto) values ('%s', '%s')", id, replyto)

	_, err := db.Exec(query)

	CheckError(err, "Could not insert local reply query")

	query = fmt.Sprintf("select inreplyto from replies where id='%s'", replyto)

	rows, err := db.Query(query)

	CheckError(err, "Could not query select inreplyto")

	defer rows.Close()

	for rows.Next() {
		var val string
		rows.Scan(&val)
		if val == "" {
			updated := time.Now().Format(time.RFC3339)			
			query := fmt.Sprintf("update activitystream set updated='%s' where id='%s'", updated, replyto)

			_, err := db.Exec(query)

			CheckError(err, "error with updating replyto updated at date")
		}
	}
}

func writeObjectReplyToDB(db *sql.DB, obj ObjectBase) {
	for i, e := range obj.InReplyTo {
		if(i == 0 || IsReplyInThread(db, obj.InReplyTo[0].Id, e.Id)){
			query := fmt.Sprintf("insert into replies (id, inreplyto) values ('%s', '%s')", obj.Id, e.Id)
			_, err := db.Exec(query)
			
			if err != nil{
				fmt.Println("error inserting replies")
				panic(err)			
			}
		}

		update := true
		for _, e := range obj.Option {
			if e == "sage" || e == "nokosage" {
				update = false
				break
			}
		}
		
		if update {
			WriteObjectUpdatesToDB(db, e)
		}			
	}
}


func WriteWalletToDB(db *sql.DB, obj ObjectBase) {
	for _, e := range obj.Option { 	
		if e == "wallet" {
			for _, e := range obj.Wallet {
				query := fmt.Sprintf("insert into wallet (id, type, address) values ('%s', '%s', '%s')", obj.Id ,e.Type, e.Address)
				_, err := db.Exec(query)

				CheckError(err, "error with write wallet query")
			}
			return 
		}
	}
}

func writeActivitytoDB(db *sql.DB, obj ObjectBase) {

	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)		
	
	query := fmt.Sprintf("insert into activitystream (id, type, name, content, published, updated, attributedto, actor) values ('%s', '%s', E'%s', E'%s', '%s', '%s', E'%s', '%s')", obj.Id ,obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor.Id)

	_, e := db.Exec(query)
	
	if e != nil{
		fmt.Println("error inserting new activity")
		panic(e)			
	}	
}

func writeActivitytoDBWithAttachment(db *sql.DB, obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) {
	
	obj.Name = EscapeString(obj.Name)
	obj.Content = EscapeString(obj.Content)
	obj.AttributedTo = EscapeString(obj.AttributedTo)
	
	query := fmt.Sprintf("insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor) values ('%s', '%s', E'%s', E'%s', '%s', '%s', '%s', '%s', E'%s', '%s')", obj.Id ,obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor.Id)

	_, e := db.Exec(query)
	
	if e != nil{
		fmt.Println("error inserting new activity with attachment")
		panic(e)			
	}	
}

func writeAttachmentToDB(db *sql.DB, obj ObjectBase) {
	query := fmt.Sprintf("insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%d');", obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	
	_, e := db.Exec(query)
	
	if e != nil{
		fmt.Println("error inserting new attachment")
		panic(e)			
	}
}

func WritePreviewToDB(db *sql.DB, obj NestedObjectBase) {
	query := fmt.Sprintf("insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%d');", obj.Id ,obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	
	_, e := db.Exec(query)
	
	if e != nil{
		fmt.Println("error inserting new attachment")
		panic(e)			
	}
}

func GetActivityFromDB(db *sql.DB, id string) Collection {
	var nColl Collection
	var result []ObjectBase

	query := fmt.Sprintf("SELECT actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor FROM activitystream WHERE id='%s' ORDER BY updated asc;", id)

	rows, err := db.Query(query)

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string	
		var previewID	string
		
		err = rows.Scan(&nColl.Actor, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = &actor

		post.Replies = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)		

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetObjectFromDB(db *sql.DB, actor Actor) Collection {
	var nColl Collection
	var result []ObjectBase

	query := fmt.Sprintf("SELECT id, name, content, type, published, updated, attributedto, attachment, preview, actor FROM activitystream WHERE actor='%s' AND id IN (SELECT id FROM replies WHERE inreplyto='') AND type='Note' ORDER BY updated asc;", actor.Id)

	rows, err := db.Query(query)

	CheckError(err, "error query object from db")
	
	defer rows.Close()
	for rows.Next(){
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id)
		
		CheckError(err, "error scan object into post struct")

		post.Actor = &actor

		post.Replies = GetObjectRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl	
}

func GetInReplyToDB(db *sql.DB, parent ObjectBase) []ObjectBase {
	var result []ObjectBase

	query := fmt.Sprintf("SELECT inreplyto FROM replies WHERE id ='%s'", parent.Id)

	rows, err := db.Query(query)

	CheckError(err, "error with inreplyto db query")

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		rows.Scan(&post.Id)

		result = append(result, post)
	}

	return result
}


func GetObjectRepliesDB(db *sql.DB, parent ObjectBase) *CollectionBase {

	var nColl CollectionBase
	var result []ObjectBase
	
	query := fmt.Sprintf("SELECT id, name, content, type, published, attributedto, attachment, preview, actor FROM activitystream WHERE id IN (SELECT id FROM replies WHERE inreplyto='%s') AND type='Note' ORDER BY published asc;", parent.Id)

	rows, err := db.Query(query)

	CheckError(err, "error with replies db query")	

	defer rows.Close()	
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)
		
		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)


		CheckError(err, "error with replies db scan")

		post.Actor = &actor
		
		post.Replies = GetObjectRepliesRepliesDB(db, post)

		post.Replies.TotalItems, post.Replies.TotalImgs = GetObjectRepliesDBCount(db, post)		
		
		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	nColl.OrderedItems = result

	remoteCollection := GetObjectRepliesRemote(db, parent)

	for _, e := range remoteCollection.OrderedItems {
		
		nColl.OrderedItems = append(nColl.OrderedItems, e)
		
	}

	return &nColl
}

func GetObjectRepliesRemote(db *sql.DB, parent ObjectBase) CollectionBase {
	var nColl CollectionBase
	var result []ObjectBase		
	query := fmt.Sprintf("select id from replies where id not in (select id from activitystream) and inreplyto='%s'", parent.Id)

	rows, err := db.Query(query)

	CheckError(err, "could not get remote id query")

	defer rows.Close()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		
		coll := GetCollectionFromID(id)

		for _, e := range coll.OrderedItems {
			result = append(result, e)
		}
	}

	nColl.OrderedItems = result

	return nColl
}

func GetObjectRepliesRepliesDB(db *sql.DB, parent ObjectBase) *CollectionBase {

	var nColl CollectionBase
	var result []ObjectBase

	query := fmt.Sprintf("SELECT id, name, content, type, published, attributedto, attachment, preview, actor FROM activitystream WHERE id IN (SELECT id FROM replies WHERE inreplyto='%s') AND type='Note' ORDER BY published asc;", parent.Id)

	rows, err := db.Query(query)

	CheckError(err, "error with replies replies db query")

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string		

		post.InReplyTo = append(post.InReplyTo, parent)

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id)


		CheckError(err, "error with replies replies db scan")

		post.Actor = &actor

		post.Attachment = GetObjectAttachment(db, attachID)

		post.Preview = GetObjectPreview(db, previewID)				

		result = append(result, post)			
	}

	remoteCollection := GetObjectRepliesRemote(db, parent)

	for _, e := range remoteCollection.OrderedItems {
		
		nColl.OrderedItems = append(nColl.OrderedItems, e)

	}	

	nColl.OrderedItems = result

	return &nColl
}

func GetObjectRepliesDBCount(db *sql.DB, parent ObjectBase) (int, int) {

	var countId int
	var countImg int 
	
	query := fmt.Sprintf("SELECT COUNT(id) FROM replies WHERE inreplyto ='%s' and id in (select id from activitystream where type='Note');", parent.Id)
	
	rows, err := db.Query(query)
	
	CheckError(err, "error with replies count db query")

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&countId)
		
		if err !=nil{
			fmt.Println("error with replies count db scan")
		}
	}

	query = fmt.Sprintf("SELECT COUNT(attachment) FROM activitystream WHERE id IN (SELECT id FROM replies WHERE inreplyto ='%s') AND attachment != '';", parent.Id)
	
	rows, err = db.Query(query)

	CheckError(err, "error with select attachment count db query")
	
	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&countImg)
		
		if err !=nil{
			fmt.Println("error with replies count db scan")
		}
	}	

	return countId, countImg
}

func GetObjectAttachment(db *sql.DB, id string) []ObjectBase {

	var attachments []ObjectBase	
	
	query := fmt.Sprintf("SELECT id, type, name, href, mediatype, size, published FROM activitystream WHERE id='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "could not select object attachment query")

	defer rows.Close()
	for rows.Next() {
		var attachment = new(ObjectBase)

		err = rows.Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published)
		if err !=nil{
			fmt.Println("error with attachment db query")
			panic(err)
		}

		attachments = append(attachments, *attachment)
	}

	return attachments
}

func GetObjectPreview(db *sql.DB, id string) *NestedObjectBase {

	var preview NestedObjectBase
	
	query := fmt.Sprintf("SELECT id, type, name, href, mediatype, size, published FROM activitystream WHERE id='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "could not select object preview query")	

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published)
	}

	return &preview
}

func GetObjectPostsTotalDB(db *sql.DB, actor Actor) int{

	count := 0
	query := fmt.Sprintf("SELECT COUNT(id) FROM activitystream WHERE actor='%s' AND id IN (SELECT id FROM replies WHERE inreplyto='' AND type='Note');", actor.Id)

	rows, err := db.Query(query)

	CheckError(err, "could not select post total count query")		

	defer rows.Close()	
	for rows.Next() {
		err = rows.Scan(&count)
		CheckError(err, "error with total post db scan")
	}
	
	return count
}

func GetObjectImgsTotalDB(db *sql.DB, actor Actor) int{

	count := 0
	query := fmt.Sprintf("SELECT COUNT(attachment) FROM activitystream WHERE actor='%s' AND id IN (SELECT id FROM replies WHERE inreplyto='' AND type='Note' );", actor.Id)

	rows, err := db.Query(query)

	CheckError(err, "error with posts total db query")			

	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&count)

		CheckError(err, "error with total post db scan")
	}
	
	return count
}


func DeleteAttachmentFromFile(db *sql.DB, id string) {
	
	var query = fmt.Sprintf("select href, type from activitystream where id in (select attachment from activitystream where id='%s')", id)

	rows, err := db.Query(query)

	CheckError(err, "error query delete attachment")				

	defer rows.Close()
	for rows.Next() {
		var href string
		var _type string
		err := rows.Scan(&href, &_type)
		href = strings.Replace(href, Domain + "/", "", 1)

		CheckError(err, "error scanning delete attachment")

		if _type != "Tombstone" {
			_, err = os.Stat(href)
			CheckError(err, "err removing file from system")
			if err == nil {
				os.Remove(href)
			}	
		}

	}

	DeleteAttachmentFromDB(db, id)
}


func DeleteAttachmentRepliesFromDB(db *sql.DB, id string) {
	var query = fmt.Sprintf("select id from activitystream where id (select id from replies where inreplyto='%s');", id)
	
	rows, err := db.Query(query)

	CheckError(err, "error query delete attachment replies")

	defer rows.Close()	
	for rows.Next() {
		var attachment string

		err := rows.Scan(&attachment)

		CheckError(err, "error scanning delete attachment")
		
		DeleteAttachmentFromFile(db, attachment)
	}	
}


func DeleteAttachmentFromDB(db *sql.DB, id string) {
	datetime := time.Now().Format(time.RFC3339)

	var query = fmt.Sprintf("update activitystream set type='Tombstone', mediatype='image/png', href='%s', name='', content='', attributedto='deleted', updated='%s', deleted='%s' where id in (select attachment from activitystream where id='%s');", Domain + "/public/removed.png", datetime, datetime, id)

	_, err := db.Exec(query)

	CheckError(err, "error with delete attachment")	
}


func DeleteObjectFromDB(db *sql.DB, id string) {
	datetime := time.Now().Format(time.RFC3339)
	var query = fmt.Sprintf("update activitystream set type='Tombstone', name='', content='', attributedto='deleted', updated='%s', deleted='%s' where id='%s';", datetime, datetime, id)

	_, err := db.Exec(query)

	CheckError(err, "error with delete object")	
}

func DeleteObjectRepliesFromDB(db *sql.DB, id string) {
	datetime := time.Now().Format(time.RFC3339)	
	var query = fmt.Sprintf("update activitystream set type='Tombstone', name='', content='', attributedto='deleted' updated='%s', deleted='%s' where id in (select id from replies where inreplyto='%s');", datetime, datetime, id)

	_, err := db.Exec(query)
	CheckError(err, "error with delete object replies")		
}

func DeleteObject(db *sql.DB, id string) {
	
	if(!IsIDLocal(db, id)) {
		return
	}
	
	DeleteObjectFromDB(db, id)
	DeleteReportActivity(db, id)	
	DeleteAttachmentFromFile(db, id)	
}

func DeleteObjectAndReplies(db *sql.DB, id string) {
	
	if(!IsIDLocal(db, id)) {
		return
	}
	
	DeleteObjectFromDB(db, id)
	DeleteReportActivity(db, id)	
	DeleteAttachmentFromFile(db, id)		
	DeleteObjectRepliesFromDB(db, id)
	DeleteAttachmentRepliesFromDB(db, id)
}

func GetRandomCaptcha(db *sql.DB) string{
	query := fmt.Sprintf("select identifier from verification where type='captcha' order by random() limit 1")
	rows, err := db.Query(query)

	CheckError(err, "could not get captcha")

	var verify string

	defer rows.Close()
	
	rows.Next()
	err = rows.Scan(&verify)
	
	CheckError(err, "Could not get verify captcha")

	return verify
}

func GetCaptchaTotal(db *sql.DB) int{
	query := fmt.Sprintf("select count(*) from verification where type='captcha'")
	rows, err := db.Query(query)
	
	CheckError(err, "could not get query captcha total")	

	defer rows.Close()
	
	var count int
	for rows.Next(){
		if err := rows.Scan(&count); err != nil{
			CheckError(err, "could not get captcha total")
		}
	}

	return count
}

func GetCaptchaCodeDB(db *sql.DB, verify string) string {
	
	query := fmt.Sprintf("select code from verification where identifier='%s' limit 1", verify)
	rows, err := db.Query(query)

	CheckError(err, "could not get captcha verifciation")

	defer rows.Close()

	var code string
	
	rows.Next()
	err = rows.Scan(&code)

	CheckError(err, "Could not get verification captcha")

	return code
}

func GetActorAuth(db *sql.DB, actor string) []string {
	query := fmt.Sprintf("select type from actorauth where board='%s'", actor)

	rows, err := db.Query(query)

	CheckError(err, "could not get actor auth")	

	defer rows.Close()	

	var auth []string
	
	for rows.Next() {
		var e string
		err = rows.Scan(&e)

		CheckError(err, "could not get actor auth row scan")		

		auth = append(auth, e)
	}

	return auth
}

func DeleteCaptchaCodeDB(db *sql.DB, verify string) {
	query := fmt.Sprintf("delete from verification where identifier='%s'", verify)

	_, err := db.Exec(query);

	CheckError(err, "could not delete captcah code db")

	os.Remove("./" + verify)
}

func EscapeString(text string) string {
	re := regexp.MustCompile("(?i)(n)+(\\s+)?(i)+(\\s+)?(g)+(\\s+)?(e)+?(\\s+)?(r)+(\\s+)?")
	text = re.ReplaceAllString(text, "I love black people")
	re = regexp.MustCompile("(?i)(n)+(\\s+)?(i)+(\\s+)?(g)(\\s+)?(g)+(\\s+)?")
	text = re.ReplaceAllString(text, "I love black people")		
	text = strings.Replace(text, "'", `''`, -1)
	text = strings.Replace(text, "<", "&lt;", -1)
	return text
}

func GetActorReportedTotal(db *sql.DB, id string) int {
	query := fmt.Sprintf("select count(id) from reported where board='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "error getting actor reported total query")

	defer rows.Close()

	var count int
	for rows.Next() {
		rows.Scan(&count)
	}
	
	return count
}

func GetActorReportedDB(db *sql.DB, id string) []ObjectBase {
	var nObj []ObjectBase

	query := fmt.Sprintf("select id, count from reported where board='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "error getting actor reported query")

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		rows.Scan(&obj.Id, &obj.Size)

		nObj = append(nObj, obj)
	}

	return nObj
}
