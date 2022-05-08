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
	"os"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func (actor Actor) AddFollower(follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`
	_, err := config.DB.Exec(query, actor.Id, follower)
	return util.MakeError(err, "AddFollwer")
}

func (actor Actor) ActivitySign(signature string) (string, error) {
	var file string

	query := `select file from publicKeyPem where id=$1 `
	if err := config.DB.QueryRow(query, actor.PublicKey.Id).Scan(&file); err != nil {
		return "", util.MakeError(err, "ActivitySign")
	}

	file = strings.ReplaceAll(file, "public.pem", "private.pem")

	_, err := os.Stat(file)
	if err != nil {
		fmt.Println(`\n Unable to locate private key. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.Name + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return "", util.MakeError(err, "ActivitySign")
	}

	var publickey []byte

	if publickey, err = ioutil.ReadFile(file); err != nil {
		return "", util.MakeError(err, "ActivitySign")
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
		return util.MakeError(err, "DeleteCache")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase
		if err := rows.Scan(&obj.Id); err != nil {
			return util.MakeError(err, "DeleteCache")
		}

		if err := obj.Delete(); err != nil {
			return util.MakeError(err, "DeleteCache")
		}
	}

	return nil
}

func (actor Actor) GetAutoSubscribe() (bool, error) {
	var subscribed bool

	query := `select autosubscribe from actor where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&subscribed); err != nil {
		return false, util.MakeError(err, "GetAutoSubscribe")
	}

	return subscribed, nil
}

func (actor Actor) GetCollectionType(nType string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc`
	rows, err := config.DB.Query(query, actor.Id, nType)
	if err != nil {
		return nColl, util.MakeError(err, "GetCollectionType")
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
			return nColl, util.MakeError(err, "GetCollectionType")
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionType")
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionType")
		}

		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionType")
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
		return nColl, util.MakeError(err, "GetCollectionTypeLimit")
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
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
		}

		post.Preview, err = post.Preview.GetPreview()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
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
		return followerCollection, util.MakeError(err, "GetFollow")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followerCollection, util.MakeError(err, "GetFollow")
		}

		followerCollection = append(followerCollection, obj)
	}

	return followerCollection, nil
}

func (actor Actor) GetFollowingTotal() (int, error) {
	var following int

	query := `select count(following) from following where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&following); err != nil {
		return following, util.MakeError(err, "GetFollowingTotal")
	}

	return following, nil
}

func (actor Actor) GetFollowersTotal() (int, error) {
	var followers int

	query := `select count(follower) from follower where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&followers); err != nil {
		return followers, util.MakeError(err, "GetFollowersTotal")
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
		return util.MakeError(err, "GetFollowersResp")
	}

	following.Items, err = actor.GetFollow()

	if err != nil {
		return util.MakeError(err, "GetFollowersResp")
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
		return util.MakeError(err, "GetFollowingResp")
	}

	following.Items, err = actor.GetFollowing()

	if err != nil {
		return util.MakeError(err, "GetFollowingResp")
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "GetFollowingResp")
}

func (actor Actor) GetFollowing() ([]ObjectBase, error) {
	var followingCollection []ObjectBase

	query := `select following from following where id=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return followingCollection, util.MakeError(err, "GetFollowing")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followingCollection, util.MakeError(err, "GetFollowing")
		}

		followingCollection = append(followingCollection, obj)
	}

	return followingCollection, nil
}

func (actor Actor) GetInfoResp(ctx *fiber.Ctx) error {
	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err := ctx.Write(enc)

	return util.MakeError(err, "GetInfoResp")
}

func (actor Actor) GetCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nColl, util.MakeError(err, "GetCollection")
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
			return nColl, util.MakeError(err, "GetCollection")
		}

		post.Actor = actor.Id

		post.Replies, post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetReplies()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollection")
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollection")
		}

		post.Preview, err = post.Preview.GetPreview()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollection")
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetRecentPosts() ([]ObjectBase, error) {
	var collection []ObjectBase

	query := `select id, actor, content, published, attachment from activitystream where actor=$1 and type='Note' union select id, actor, content, published, attachment from cacheactivitystream where actor in (select follower from follower where id=$1) and type='Note' order by published desc limit 20`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return collection, util.MakeError(err, "GetRecentPosts")
	}

	defer rows.Close()
	for rows.Next() {
		var nObj ObjectBase
		var attachment ObjectBase

		err := rows.Scan(&nObj.Id, &nObj.Actor, &nObj.Content, &nObj.Published, &attachment.Id)

		if err != nil {
			return collection, util.MakeError(err, "GetRecentPosts")
		}

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
		return nObj, util.MakeError(err, "GetReported")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		err := rows.Scan(&obj.Id, &obj.Size, &obj.Content)

		if err != nil {
			return nObj, util.MakeError(err, "GetReported")
		}

		nObj = append(nObj, obj)
	}

	return nObj, nil
}

func (actor Actor) GetReportedTotal() (int, error) {
	var count int

	query := `select count(id) from reported where board=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return 0, util.MakeError(err, "GetReportedTotal")
	}

	return count, nil
}

func (actor Actor) GetAllArchive(offset int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`
	rows, err := config.DB.Query(query, actor.Id, offset)

	if err != nil {
		return nColl, util.MakeError(err, "GetAllArchive")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, util.MakeError(err, "GetAllArchive")
		}

		post.Replies, _, _, err = post.GetReplies()

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) IsAlreadyFollowing(follow string) (bool, error) {
	followers, err := actor.GetFollowing()

	if err != nil {
		return false, util.MakeError(err, "IsAlreadyFollowing")
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
		return false, util.MakeError(err, "IsAlreadyFollower")
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
		return util.MakeError(err, "SetActorAutoSubscribeDB")
	}

	query := `update actor set autosubscribe=$1 where id=$2`
	_, err = config.DB.Exec(query, !current, actor.Id)

	return util.MakeError(err, "SetActorAutoSubscribeDB")
}

func (actor Actor) GetOutbox(ctx *fiber.Ctx) error {
	var collection Collection

	c, err := actor.GetCollection()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	collection.OrderedItems = c.OrderedItems
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = actor

	collection.TotalItems, err = actor.GetPostTotal()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	collection.TotalImgs, err = actor.GetImgTotal()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	enc, _ := json.Marshal(collection)
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "GetOutbox")
}

func (actor Actor) UnArchiveLast() error {
	col, err := actor.GetCollectionTypeLimit("Archive", 1)

	if err != nil {
		return util.MakeError(err, "UnArchiveLast")
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := k.UpdateType("Note"); err != nil {
				return util.MakeError(err, "UnArchiveLast")
			}
		}

		if err := e.UpdateType("Note"); err != nil {
			return util.MakeError(err, "UnArchiveLast")
		}
	}

	return nil
}

func (actor Actor) IsLocal() (bool, error) {
	actor, err := GetActorFromDB(actor.Id)
	return actor.Id != "", util.MakeError(err, "IsLocal")
}

func (actor Actor) GetCatalogCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 165`
	if rows, err = config.DB.Query(query, actor.Id); err != nil {
		return nColl, util.MakeError(err, "GetCatalogCollection")
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
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = replies

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()

		if err != nil {
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()

		if err != nil {
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

		post.Preview, err = post.Preview.GetPreview()

		if err != nil {
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

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
		return nColl, util.MakeError(err, "GetCollectionPage")
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
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		post.Actor = actor.Id

		post.Replies, post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesLimit(5)

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		post.Attachment, err = post.Attachment[0].GetAttachment()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		post.Preview, err = post.Preview.GetPreview()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.TotalItems = count

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) WantToServePage(page int) (Collection, error) {
	var collection Collection
	var err error

	if page > config.PostCountPerPage {
		return collection, errors.New("above page limit")
	}

	if collection, err = actor.GetCollectionPage(page); err != nil {
		return collection, util.MakeError(err, "WantToServePage")
	}

	collection.Actor = actor

	return collection, nil
}

func (actor Actor) GetImgTotal() (int, error) {
	var count int

	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.MakeError(err, "GetImgTotal")
	}

	return count, nil
}

func (actor Actor) GetPostTotal() (int, error) {
	var count int

	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.MakeError(err, "GetPostTotal")
	}

	return count, nil
}
