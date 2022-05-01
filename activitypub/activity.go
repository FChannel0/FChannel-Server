package activitypub

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

// False positive for application/ld+ld, application/activity+ld, application/json+json
var activityRegexp = regexp.MustCompile("application\\/(ld|json|activity)((\\+(ld|json))|$)")

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

func ActivitySign(actor Actor, signature string) (string, error) {
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

func DeleteReportActivity(id string) error {
	query := `delete from reported where id=$1`

	_, err := config.DB.Exec(query, id)
	return err
}

func GetActivityFromDB(id string) (Collection, error) {
	var nColl Collection
	var nActor Actor
	var result []ObjectBase

	nColl.Actor = &nActor

	query := `select  actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from  activitystream where id=$1 order by updated asc`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&nColl.Actor.Id, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		var err error
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

func GetActivityFromJson(ctx *fiber.Ctx) (Activity, error) {

	var respActivity ActivityRaw
	var nActivity Activity
	var nType string

	if err := json.Unmarshal(ctx.Body(), &respActivity); err != nil {
		return nActivity, err
	}

	if res, err := HasContextFromJson(respActivity.AtContextRaw.Context); err == nil && res {
		var jObj ObjectBase

		if respActivity.Type == "Note" {
			jObj, err = GetObjectFromJson(ctx.Body())
			if err != nil {
				return nActivity, err
			}

			nType = "Create"
		} else {
			jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
			if err != nil {
				return nActivity, err
			}

			nType = respActivity.Type
		}

		actor, err := GetActorFromJson(respActivity.ActorRaw)
		if err != nil {
			return nActivity, err
		}

		to, err := GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return nActivity, err
		}

		cc, err := GetToFromJson(respActivity.CcRaw)
		if err != nil {
			return nActivity, err
		}

		nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		nActivity.Type = nType
		nActivity.Actor = &actor
		nActivity.Published = respActivity.Published
		nActivity.Auth = respActivity.Auth

		if len(to) > 0 {
			nActivity.To = to
		}

		if len(cc) > 0 {
			nActivity.Cc = cc
		}

		nActivity.Name = respActivity.Name
		nActivity.Object = &jObj
	} else if err != nil {
		return nActivity, err
	}

	return nActivity, nil
}

func HasContextFromJson(context []byte) (bool, error) {
	var generic interface{}

	err := json.Unmarshal(context, &generic)
	if err != nil {
		return false, err
	}

	hasContext := false

	switch generic.(type) {
	case []interface{}:
		var arrContext AtContextArray
		err = json.Unmarshal(context, &arrContext.Context)
		if len(arrContext.Context) > 0 {
			if arrContext.Context[0] == "https://www.w3.org/ns/activitystreams" {
				hasContext = true
			}
		}
		break

	case string:
		var arrContext AtContextString
		err = json.Unmarshal(context, &arrContext.Context)
		if arrContext.Context == "https://www.w3.org/ns/activitystreams" {
			hasContext = true
		}
		break
	}

	return hasContext, err
}

func IsActivityLocal(activity Activity) (bool, error) {

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

func ProcessActivity(activity Activity) error {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if res, err := GetActorFromDB(e); err == nil && res.Id != "" {
				fmt.Println("actor is in the database")
			} else if err != nil {
				return err
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

func ReportActivity(id string, reason string) (bool, error) {
	if res, err := IsIDLocal(id); err == nil && !res {
		// TODO: not local error
		return false, nil
	} else if err != nil {
		return false, err
	}

	actor, err := GetActivityFromDB(id)
	if err != nil {
		return false, err
	}

	query := `select count from reported where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return false, err
		}
	}

	if count < 1 {
		query = `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		_, err := config.DB.Exec(query, id, 1, actor.Actor.Id, reason)
		if err != nil {
			return false, err
		}
	} else {
		count = count + 1
		query = `update reported set count=$1 where id=$2`

		_, err := config.DB.Exec(query, count, id)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func WriteActivitytoCache(obj ObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := config.DB.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteActivitytoCacheWithAttachment(obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`

	rows, err := config.DB.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteActivitytoDB(obj ObjectBase) error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func WriteActivitytoDBWithAttachment(obj ObjectBase, attachment ObjectBase, preview NestedObjectBase) {

	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	if e != nil {
		fmt.Println("error inserting new activity with attachment")
		panic(e)
	}
}

func WriteAttachmentToCache(obj ObjectBase) error {
	query := `select id from cacheactivitystream where id=$1`

	rows, err := config.DB.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}

func WriteAttachmentToDB(obj ObjectBase) {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)

	if e != nil {
		fmt.Println("error inserting new attachment")
		panic(e)
	}
}

func WritePreviewToCache(obj NestedObjectBase) error {
	query := `select id from cacheactivitystream where id=$1`

	rows, err := config.DB.Query(query, obj.Id)
	if err != nil {
		return err
	}
	defer rows.Close()

	var id string
	rows.Next()
	err = rows.Scan(&id)
	if err != nil {
		return err
	} else if id != "" {
		return nil // TODO: error?
	}

	if obj.Updated.IsZero() {
		obj.Updated = obj.Published
	}

	query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}

func WritePreviewToDB(obj NestedObjectBase) error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}
