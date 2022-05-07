package activitypub

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

func (actor Actor) AddFollower(follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`
	_, err := config.DB.Exec(query, actor.Id, follower)
	return err
}

func (actor Actor) ActivitySign(signature string) (string, error) {
	query := `select file from publicKeyPem where id=$1 `

	rows, err := config.DB.Query(query, actor.PublicKey.Id)
	if err != nil {
		return "", err
	}

	defer rows.Close()

	var file string
	rows.Next()
	rows.Scan(&file)

	file = strings.ReplaceAll(file, "public.pem", "private.pem")
	_, err = os.Stat(file)
	if err != nil {
		fmt.Println(`\n Unable to locate private key. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.Name + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return "", errors.New("unable to locate private key")
	}

	publickey, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(publickey)

	pub, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	rng := crand.Reader
	hashed := sha256.New()
	hashed.Write([]byte(signature))
	cipher, _ := rsa.SignPKCS1v15(rng, pub, crypto.SHA256, hashed.Sum(nil))

	return base64.StdEncoding.EncodeToString(cipher), nil
}

func (actor Actor) DeleteCache() error {
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1)`

	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var obj ObjectBase
		if err := rows.Scan(&obj.Id); err != nil {
			return err
		}

		if err := obj.Delete(); err != nil {
			return err
		}
	}

	return nil
}

func (actor Actor) GetAutoSubscribe() (bool, error) {
	var subscribed bool

	query := `select autosubscribe from actor where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&subscribed); err != nil {
		return false, err
	}

	return subscribed, nil
}

func (actor Actor) GetCollectionType(nType string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc`

	rows, err := config.DB.Query(query, actor.Id, nType)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase
		post.Attachment = append(post.Attachment, attch)

		var prev NestedObjectBase
		post.Preview = &prev

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		var err error

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()
		if err != nil {
			return nColl, err
		}

		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionTypeLimit(nType string, limit int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc limit $3`

	rows, err := config.DB.Query(query, actor.Id, nType, limit)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase
		post.Attachment = append(post.Attachment, attch)

		var prev NestedObjectBase
		post.Preview = &prev

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		var err error
		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, err
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()
		if err != nil {
			return nColl, err
		}

		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetFollow() ([]ObjectBase, error) {
	var followerCollection []ObjectBase

	query := `select follower from follower where id=$1`

	rows, err := config.DB.Query(query, actor.Id)
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

func (actor Actor) GetFollowingTotal() (int, error) {
	var following int

	query := `select count(following) from following where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&following); err != nil {
		return following, err
	}

	return following, nil
}

func (actor Actor) GetFollowersTotal() (int, error) {
	var followers int

	query := `select count(follower) from follower where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&followers); err != nil {
		return followers, err
	}

	return followers, nil
}

func (actor Actor) GetFollowersResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowingTotal()
	if err != nil {
		return err
	}

	following.Items, err = actor.GetFollow()
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)
	return err
}

func (actor Actor) GetFollowingResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowingTotal()
	if err != nil {
		return err
	}

	following.Items, err = actor.GetFollowing()
	if err != nil {
		return err
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return err
}

func (actor Actor) GetFollowing() ([]ObjectBase, error) {
	var followingCollection []ObjectBase
	query := `select following from following where id=$1`

	rows, err := config.DB.Query(query, actor.Id)
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

func (actor Actor) GetInfoResp(ctx *fiber.Ctx) error {
	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err := ctx.Write(enc)

	return err
}

func (actor Actor) GetCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`

	rows, err := config.DB.Query(query, actor.Id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase
		post.Attachment = append(post.Attachment, attch)

		var prev NestedObjectBase
		post.Preview = &prev

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = post.GetReplies()
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = post.Attachment[0].GetAttachment()
		if err != nil {
			return nColl, err
		}

		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return nColl, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetRecentPosts() ([]ObjectBase, error) {
	var collection []ObjectBase

	query := `select id, actor, content, published, attachment from activitystream where actor=$1 and type='Note' union select id, actor, content, published, attachment from cacheactivitystream where actor in (select follower from follower where id=$1) and type='Note' order by published desc limit 20`

	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		log.Println("Could not get recent posts")
		return collection, err
	}

	defer rows.Close()
	for rows.Next() {
		var nObj ObjectBase
		var attachment ObjectBase
		rows.Scan(&nObj.Id, &nObj.Actor, &nObj.Content, &nObj.Published, &attachment.Id)

		isOP, _ := nObj.CheckIfOP()

		nObj.Attachment, _ = attachment.GetAttachment()

		if !isOP {
			var reply ObjectBase
			reply.Id = nObj.Id
			nObj.InReplyTo = append(nObj.InReplyTo, reply)
		}

		collection = append(collection, nObj)
	}

	return collection, nil
}

func (actor Actor) GetReported() ([]ObjectBase, error) {
	var nObj []ObjectBase

	query := `select id, count, reason from reported where board=$1`

	rows, err := config.DB.Query(query, actor.Id)
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

func (actor Actor) GetReportedTotal() (int, error) {
	var count int

	query := `select count(id) from reported where board=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func (actor Actor) GetAllArchive(offset int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`

	rows, err := config.DB.Query(query, actor.Id, offset)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, err
		}

		post.Replies, _, _, err = post.GetReplies()

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) IsAlreadyFollowing(follow string) (bool, error) {
	followers, err := actor.GetFollowing()
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

func (actor Actor) IsAlreadyFollower(follow string) (bool, error) {
	followers, err := actor.GetFollow()
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

func (actor Actor) SetActorAutoSubscribeDB() error {
	current, err := actor.GetAutoSubscribe()
	if err != nil {
		return err
	}

	query := `update actor set autosubscribe=$1 where id=$2`

	_, err = config.DB.Exec(query, !current, actor.Id)
	return err
}

func (actor Actor) GetOutbox(ctx *fiber.Ctx) error {

	var collection Collection

	c, err := actor.GetCollection()
	if err != nil {
		return err
	}
	collection.OrderedItems = c.OrderedItems

	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = actor

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

func (actor Actor) UnArchiveLast() error {
	col, err := actor.GetCollectionTypeLimit("Archive", 1)
	if err != nil {
		return err
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := k.UpdateType("Note"); err != nil {
				return err
			}
		}

		if err := e.UpdateType("Note"); err != nil {
			return err
		}
	}

	return nil
}

func GetActorByNameFromDB(name string) (Actor, error) {
	var nActor Actor
	var publicKeyPem string

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where name=$1`
	err := config.DB.QueryRow(query, name).Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)
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

func GetActorFollowNameFromPath(path string) string {
	var actor string

	re := regexp.MustCompile("f\\w+-")

	actor = re.FindString(path)

	actor = strings.Replace(actor, "f", "", 1)
	actor = strings.Replace(actor, "-", "", 1)

	return actor
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

func GetActorsFollowPostFromId(actors []string, id string) (Collection, error) {
	var collection Collection

	for _, e := range actors {
		obj := ObjectBase{Id: e + "/" + id}
		tempCol, err := obj.GetCollectionFromPath()
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
	obj := ObjectBase{Id: config.Domain + "" + path}
	collection, err := obj.GetCollectionFromPath()
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

func SetActorFollowerDB(activity Activity) (Activity, error) {
	var query string
	alreadyFollow, err := activity.Actor.IsAlreadyFollower(activity.Object.Actor)
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

func (obj ObjectBase) WriteActorObjectReplyToDB() error {
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

func (obj ObjectBase) WriteActorObjectToCache() (ObjectBase, error) {
	if res, err := util.IsPostBlacklist(obj.Content); err == nil && res {
		fmt.Println("\n\nBlacklist post blocked\n\n")
		return obj, nil
	} else if err != nil {
		return obj, err
	}

	if len(obj.Attachment) > 0 {
		if res, err := obj.IsLocal(); err == nil && res {
			return obj, err
		} else if err != nil {
			return obj, err
		}

		if obj.Preview.Href != "" {
			obj.Preview.WritePreviewCache()
		}

		for i, _ := range obj.Attachment {
			obj.Attachment[i].WriteAttachmentCache()
			obj.WriteCacheWithAttachment(obj.Attachment[i])
		}

	} else {
		if err := obj.WriteCache(); err != nil {
			return obj, err
		}
	}

	obj.WriteActorObjectReplyToDB()

	if obj.Replies.OrderedItems != nil {
		for _, e := range obj.Replies.OrderedItems {
			e.WriteActorObjectToCache()
		}
	}

	return obj, nil
}

func (actor Actor) GetCatalogCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 165`

	if rows, err = config.DB.Query(query, actor.Id); err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase
		post.Attachment = append(post.Attachment, attch)

		var prev NestedObjectBase
		post.Preview = &prev

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		post.Replies.TotalItems, post.Replies.TotalImgs, _ = post.GetRepliesCount()

		post.Attachment, _ = post.Attachment[0].GetAttachment()

		post.Preview, _ = post.Preview.GetPreview()

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionPage(page int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select count (x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 15 offset $2`

	if rows, err = config.DB.Query(query, actor.Id, page*15); err != nil {
		return nColl, err
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase
		post.Attachment = append(post.Attachment, attch)

		var prev NestedObjectBase
		post.Preview = &prev

		err = rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt, _ = post.GetRepliesLimit(5)

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, _ = post.Attachment[0].GetAttachment()

		post.Preview, _ = post.Preview.GetPreview()

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.TotalItems = count

	nColl.OrderedItems = result

	return nColl, nil
}
