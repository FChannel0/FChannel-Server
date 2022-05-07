package activitypub

import (
	"encoding/json"
	"errors"
	"fmt"
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

func DeleteReportActivity(id string) error {
	query := `delete from reported where id=$1`

	_, err := config.DB.Exec(query, id)
	return err
}

func GetActivityFromDB(id string) (Collection, error) {
	var nColl Collection
	var nActor Actor
	var result []ObjectBase

	nColl.Actor = nActor

	query := `select  actor, id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from  activitystream where id=$1 order by updated asc`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return nColl, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		if err := rows.Scan(&nColl.Actor.Id, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &post.Attachment[0].Id, &post.Preview.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		var err error

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

func (activity Activity) Report(reason string) (bool, error) {
	if res, err := activity.Object.IsLocal(); err == nil && !res {
		// TODO: not local error
		return false, nil
	} else if err != nil {
		return false, err
	}

	activityCol, err := GetActivityFromDB(activity.Object.Id)
	if err != nil {
		return false, err
	}

	query := `select count from reported where id=$1`

	rows, err := config.DB.Query(query, activity.Object.Id)
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

		_, err := config.DB.Exec(query, activity.Object.Object.Id, 1, activityCol.Actor.Id, reason)
		if err != nil {
			return false, err
		}
	} else {
		count = count + 1
		query = `update reported set count=$1 where id=$2`

		_, err := config.DB.Exec(query, count, activity.Object.Id)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (obj ObjectBase) WriteCache() error {
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

func (obj ObjectBase) WriteCacheWithAttachment(attachment ObjectBase) error {
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

	_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
	return err
}

func (obj ObjectBase) WriteWithAttachment(attachment ObjectBase) {

	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	if e != nil {
		fmt.Println("error inserting new activity with attachment")
		panic(e)
	}
}

func (obj ObjectBase) WriteAttachmentCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated.IsZero() {
			obj.Updated = obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return err
	}

	return nil
}

func (obj ObjectBase) WriteAttachment() {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)

	if e != nil {
		fmt.Println("error inserting new attachment")
		panic(e)
	}
}

func (obj NestedObjectBase) WritePreviewCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	err := config.DB.QueryRow(query, obj.Id).Scan(&id)
	if err != nil {
		if obj.Updated.IsZero() {
			obj.Updated = obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return err
	}

	return nil
}

func (obj NestedObjectBase) WritePreview() error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return err
}
