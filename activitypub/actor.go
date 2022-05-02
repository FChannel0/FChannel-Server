package activitypub

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func CreateNewActor(board string, prefName string, summary string, authReq []string, restricted bool) *Actor {
	actor := new(Actor)

	var path string
	if board == "" {
		path = config.Domain
		actor.Name = "main"
	} else {
		path = config.Domain + "/" + board
		actor.Name = board
	}

	actor.Type = "Group"
	actor.Id = fmt.Sprintf("%s", path)
	actor.Following = fmt.Sprintf("%s/following", actor.Id)
	actor.Followers = fmt.Sprintf("%s/followers", actor.Id)
	actor.Inbox = fmt.Sprintf("%s/inbox", actor.Id)
	actor.Outbox = fmt.Sprintf("%s/outbox", actor.Id)
	actor.PreferredUsername = prefName
	actor.Restricted = restricted
	actor.Summary = summary
	actor.AuthRequirement = authReq

	return actor
}

func DeleteActorCache(actorID string) error {
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1)`

	rows, err := config.DB.Query(query, actorID)

	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}

		if err := DeleteObject(id); err != nil {
			return err
		}
	}

	return nil
}

func GetActorAuth(actor string) ([]string, error) {
	var auth []string

	query := `select type from actorauth where board=$1`

	rows, err := config.DB.Query(query, actor)
	if err != nil {
		return auth, err
	}
	defer rows.Close()

	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return auth, err
		}

		auth = append(auth, e)
	}

	return auth, nil
}

func GetActorAutoSubscribeDB(id string) (bool, error) {
	query := `select autosubscribe from actor where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return false, err
	}

	var subscribed bool
	defer rows.Close()
	rows.Next()
	err = rows.Scan(&subscribed)
	return subscribed, err
}

func GetActorByNameFromDB(name string) (Actor, error) {
	var nActor Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where name=$1`

	rows, err := config.DB.Query(query, name)
	if err != nil {
		return nActor, err
	}

	var publicKeyPem string
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem); err != nil {
			return nActor, err
		}
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, err
		}
	}

	return nActor, nil
}

func GetActorCollectionDBType(actorId string, nType string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc`

	rows, err := config.DB.Query(query, actorId, nType)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = &replies

		var err error
		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorCollectionDBTypeLimit(actorId string, nType string, limit int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc limit $3`

	rows, err := config.DB.Query(query, actorId, nType, limit)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = &replies

		var err error
		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorCollectionReq(collection string) (Collection, error) {
	var nCollection Collection

	req, err := http.NewRequest("GET", collection, nil)
	if err != nil {
		return nCollection, err
	}

	// TODO: rewrite this for fiber
	pass := "FIXME"
	//_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", config.ActivityStreams)

	req.Header.Set("Authorization", "Basic "+pass)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return nCollection, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		if err := json.Unmarshal(body, &nCollection); err != nil {
			return nCollection, err
		}
	}

	return nCollection, nil
}

func GetActorFollowDB(id string) ([]ObjectBase, error) {
	var followerCollection []ObjectBase

	query := `select follower from follower where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return followerCollection, err
	}
	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followerCollection, err
		}

		followerCollection = append(followerCollection, obj)
	}

	return followerCollection, nil
}

func GetActorFollowNameFromPath(path string) string {
	var actor string

	re := regexp.MustCompile("f\\w+-")

	actor = re.FindString(path)

	actor = strings.Replace(actor, "f", "", 1)
	actor = strings.Replace(actor, "-", "", 1)

	return actor
}

func GetActorFollowTotal(id string) (int, int, error) {
	var following int
	var followers int

	query := `select count(following) from following where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&following); err != nil {
			return following, 0, err
		}
	}

	query = `select count(follower) from follower where id=$1`

	rows, err = config.DB.Query(query, id)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&followers); err != nil {
			return following, followers, err
		}

	}

	return following, followers, nil
}

func GetActorFollowers(w http.ResponseWriter, id string) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	_, following.TotalItems, err = GetActorFollowTotal(id)
	if err != nil {
		return err
	}

	following.Items, err = GetActorFollowDB(id)
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", config.ActivityStreams)
	_, err = w.Write(enc)
	return err
}

func GetActorFollowing(w http.ResponseWriter, id string) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, _, err = GetActorFollowTotal(id)
	if err != nil {
		return err
	}

	following.Items, err = GetActorFollowingDB(id)
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	w.Header().Set("Content-Type", config.ActivityStreams)
	_, err = w.Write(enc)

	return err
}

func GetActorFollowingDB(id string) ([]ObjectBase, error) {
	var followingCollection []ObjectBase
	query := `select following from following where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return followingCollection, err
	}
	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followingCollection, err
		}

		followingCollection = append(followingCollection, obj)
	}

	return followingCollection, nil
}

func GetActorFromDB(id string) (Actor, error) {
	var nActor Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where id=$1`

	var publicKeyPem string
	err := config.DB.QueryRow(query, id).Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)
	if err != nil {
		return nActor, err
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)
	if err != nil {
		return nActor, err
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, err
		}
	}

	return nActor, nil
}

func GetActorFromJson(actor []byte) (Actor, error) {
	var generic interface{}
	var nActor Actor
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

func GetActorInfo(ctx *fiber.Ctx, id string) error {
	actor, err := GetActorFromDB(id)
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err = ctx.Write(enc)

	return err
}

func GetActorInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if atFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)(www)?([\w\d-_.:]+)(/|\s+|\r|\r\n)?$`)
	mainActor := re.MatchString(path)
	if mainActor {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return "main", match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if httpFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 3 {
			if match[4] == "users" {
				return match[6], match[3]
			}

			return match[4], match[3]
		}
	}

	return "", ""
}

func GetActorObjectCollectionFromDB(actorId string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`

	rows, err := config.DB.Query(query, actorId)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return nColl, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetActorReportedDB(id string) ([]ObjectBase, error) {
	var nObj []ObjectBase

	query := `select id, count, reason from reported where board=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return nObj, err
	}

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase

		rows.Scan(&obj.Id, &obj.Size, &obj.Content)

		nObj = append(nObj, obj)
	}

	return nObj, nil
}

func GetActorReportedTotal(id string) (int, error) {
	query := `select count(id) from reported where board=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return 0, err
	}

	defer rows.Close()

	var count int
	for rows.Next() {
		rows.Scan(&count)
	}

	return count, nil
}

func GetActorsFollowPostFromId(actors []string, id string) (Collection, error) {
	var collection Collection

	for _, e := range actors {
		tempCol, err := GetObjectByIDFromDB(e + "/" + id)
		if err != nil {
			return collection, err
		}

		if len(tempCol.OrderedItems) > 0 {
			collection = tempCol
			return collection, nil
		}
	}

	return collection, nil
}

func GetActorPost(ctx *fiber.Ctx, path string) error {
	collection, err := GetCollectionFromPath(config.Domain + "" + path)
	if err != nil {
		return err
	}

	if len(collection.OrderedItems) > 0 {
		enc, err := json.MarshalIndent(collection, "", "\t")
		if err != nil {
			return err
		}

		ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		_, err = ctx.Write(enc)
		return err
	}

	return nil
}

func GetAllActorArchiveDB(id string, offset int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`

	rows, err := config.DB.Query(query, id, offset)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, err
		}

		post.Replies, _, _, err = GetObjectRepliesDB(post)

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func GetBoards() ([]Actor, error) {
	var boards []Actor

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers FROM actor`

	rows, err := config.DB.Query(query)
	if err != nil {
		return boards, err
	}

	defer rows.Close()
	for rows.Next() {
		var actor = new(Actor)

		if err := rows.Scan(&actor.Type, &actor.Id, &actor.Name, &actor.PreferredUsername, &actor.Inbox, &actor.Outbox, &actor.Following, &actor.Followers); err != nil {
			return boards, err
		}

		boards = append(boards, *actor)
	}

	return boards, nil
}

func IsActorLocal(id string) (bool, error) {
	actor, err := GetActorFromDB(id)
	return actor.Id != "", err
}

func IsAlreadyFollowing(actor string, follow string) (bool, error) {
	followers, err := GetActorFollowingDB(actor)
	if err != nil {
		return false, err
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func IsAlreadyFollower(actor string, follow string) (bool, error) {
	followers, err := GetActorFollowDB(actor)
	if err != nil {
		return false, err
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func SetActorAutoSubscribeDB(id string) error {
	current, err := GetActorAutoSubscribeDB(id)
	if err != nil {
		return err
	}

	query := `update actor set autosubscribe=$1 where id=$2`

	_, err = config.DB.Exec(query, !current, id)
	return err
}

func SetActorFollowerDB(activity Activity) (Activity, error) {
	var query string
	alreadyFollow, err := IsAlreadyFollower(activity.Actor.Id, activity.Object.Actor)
	if err != nil {
		return activity, err
	}

	activity.Type = "Reject"
	if activity.Actor.Id == activity.Object.Actor {
		return activity, nil
	}

	if alreadyFollow {
		query = `delete from follower where id=$1 and follower=$2`
		activity.Summary = activity.Object.Actor + " Unfollow " + activity.Actor.Id

		if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
			return activity, err
		}

		activity.Type = "Accept"
		return activity, err
	}

	query = `insert into follower (id, follower) values ($1, $2)`
	activity.Summary = activity.Object.Actor + " Follow " + activity.Actor.Id

	if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
		return activity, err
	}

	activity.Type = "Accept"
	return activity, nil
}

func WriteActorObjectReplyToDB(obj ObjectBase) error {
	for _, e := range obj.InReplyTo {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := config.DB.Query(query, obj.Id, e.Id)
		if err != nil {
			return err
		}

		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := config.DB.Exec(query, obj.Id, e.Id); err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		rows, err := config.DB.Query(query, obj.Id, "")
		if err != nil {
			return err
		}
		defer rows.Close()

		var id string
		rows.Next()
		rows.Scan(&id)

		if id == "" {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := config.DB.Exec(query, obj.Id, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteActorObjectToCache(obj ObjectBase) (ObjectBase, error) {
	if res, err := util.IsPostBlacklist(obj.Content); err == nil && res {
		fmt.Println("\n\nBlacklist post blocked\n\n")
		return obj, nil
	} else if err != nil {
		return obj, err
	}

	if len(obj.Attachment) > 0 {
		if res, err := IsIDLocal(obj.Id); err == nil && res {
			return obj, err
		} else if err != nil {
			return obj, err
		}

		if obj.Preview.Href != "" {
			WritePreviewToCache(*obj.Preview)
		}

		for i, _ := range obj.Attachment {
			WriteAttachmentToCache(obj.Attachment[i])
			WriteActivitytoCacheWithAttachment(obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		WriteActivitytoCache(obj)
	}

	WriteActorObjectReplyToDB(obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteActorObjectToCache(e)
		}
	}

	return obj, nil
}

func GetActorOutbox(ctx *fiber.Ctx, actor Actor) error {

	var collection Collection

	c, err := GetActorObjectCollectionFromDB(actor.Id)
	if err != nil {
		return err
	}
	collection.OrderedItems = c.OrderedItems

	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, err = GetObjectPostsTotalDB(actor)
	if err != nil {
		return err
	}

	collection.TotalImgs, err = GetObjectImgsTotalDB(actor)
	if err != nil {
		return err
	}

	enc, _ := json.Marshal(collection)

	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)
	return err
}
