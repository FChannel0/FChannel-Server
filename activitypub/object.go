package activitypub

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
)

func CheckIfObjectOP(id string) (bool, error) {
	var count int

	query := `select count(id) from replies where inreplyto='' and id=$1 `

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&count); err != nil {
		return false, err
	}

	if count > 0 {
		return true, nil
	}

	return false, nil
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]ObjectBase, *os.File, error) {
	contentType, err := util.GetFileContentType(file)
	if err != nil {
		return nil, nil, err
	}

	filename := header.Filename
	size := header.Size

	re := regexp.MustCompile(`.+/`)

	fileType := re.ReplaceAllString(contentType, "")

	tempFile, err := ioutil.TempFile("./public", "*."+fileType)
	if err != nil {
		return nil, nil, err
	}

	var nAttachment []ObjectBase
	var image ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = config.Domain + "/" + tempFile.Name()
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile, nil
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().UTC()
	nObj.Updated = time.Now().UTC()

	return nObj
}

func CreatePreviewObject(obj ObjectBase) *NestedObjectBase {
	re := regexp.MustCompile(`/.+$`)

	mimetype := re.ReplaceAllString(obj.MediaType, "")

	var nPreview NestedObjectBase

	if mimetype != "image" {
		return &nPreview
	}

	re = regexp.MustCompile(`.+/`)

	file := re.ReplaceAllString(obj.MediaType, "")

	href := util.GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = config.Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)

	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+href)

	if err := cmd.Run(); err != nil {
		// TODO: previously we would call CheckError here
		var preview NestedObjectBase
		return &preview
	}

	return &nPreview
}

func DeleteAttachmentFromDB(id string) error {
	query := `delete from activitystream where id in (select attachment from activitystream where id=$1)`

	if _, err := config.DB.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id in (select attachment from cacheactivitystream where id=$1)`

	_, err := config.DB.Exec(query, id)
	return err
}

func DeleteAttachmentFromFile(id string) error {
	query := `select href from activitystream where id in (select attachment from activitystream where id=$1)`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var href string

		if err := rows.Scan(&href); err != nil {
			return err
		}

		href = strings.Replace(href, config.Domain+"/", "", 1)

		if href != "static/notfound.png" {
			_, err = os.Stat(href)
			if err == nil {
				os.Remove(href)
			}
			return err
		}
	}

	return nil
}

func DeletePreviewFromDB(id string) error {
	query := `delete from activitystream  where id=$1`

	if _, err := config.DB.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id in (select preview from cacheactivitystream where id=$1)`

	_, err := config.DB.Exec(query, id)
	return err
}

func DeletePreviewFromFile(id string) error {
	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var href string

		if err := rows.Scan(&href); err != nil {
			return err
		}

		href = strings.Replace(href, config.Domain+"/", "", 1)

		if href != "static/notfound.png" {
			_, err = os.Stat(href)
			if err == nil {
				return os.Remove(href)
			}
			return err
		}
	}

	return nil
}

func DeleteObject(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := DeletePreviewFromDB(id); err != nil {
		return err
	}

	if err := DeleteObjectFromDB(id); err != nil {
		return err
	}

	return DeleteObjectRepliedTo(id)
}

func DeleteObjectFromDB(id string) error {
	var query = `delete from activitystream where id=$1`

	if _, err := config.DB.Exec(query, id); err != nil {
		return err
	}

	query = `delete from cacheactivitystream where id=$1`

	_, err := config.DB.Exec(query, id)
	return err
}

func DeleteObjectRepliedTo(id string) error {
	query := `delete from replies where id=$1`
	_, err := config.DB.Exec(query, id)
	return err
}

func DeleteObjectsInReplyTo(id string) error {
	query := `delete from replies where id in (select id from replies where inreplyto=$1)`
	_, err := config.DB.Exec(query, id)
	return err
}

func GetCollectionFromID(id string) (Collection, error) {
	var nColl Collection

	req, err := http.NewRequest("GET", id, nil)
	if err != nil {
		return nColl, err
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return nColl, err
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			if err := json.Unmarshal(body, &nColl); err != nil {
				return nColl, err
			}
		}
	}

	return nColl, nil
}

func GetCollectionFromPath(path string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := config.DB.Query(query, path)
	if err != nil {
		return nColl, err
	}
	defer rows.Close()

	for rows.Next() {
		var actor Actor
		var post ObjectBase
		var attachID string
		var previewID string

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		post.InReplyTo, err = GetInReplyToDB(post)
		if err != nil {
			return nColl, err
		}

		var postCnt int
		var imgCnt int
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = GetObjectRepliesCount(post)
		if err != nil {
			return nColl, err
		}

		post.Replies.TotalItems = post.Replies.TotalItems + postCnt
		post.Replies.TotalImgs = post.Replies.TotalImgs + imgCnt

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

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func GetInReplyToDB(parent ObjectBase) ([]ObjectBase, error) {
	var result []ObjectBase

	query := `select inreplyto from replies where id =$1`

	rows, err := config.DB.Query(query, parent.Id)
	if err != nil {
		return result, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id); err != nil {
			return result, err
		}

		result = append(result, post)
	}

	return result, nil
}

func GetObjectAttachment(id string) ([]ObjectBase, error) {
	var attachments []ObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return attachments, err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment = new(ObjectBase)

		if err := rows.Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published); err != nil {
			return attachments, err
		}

		attachments = append(attachments, *attachment)
	}

	return attachments, nil
}

func GetObjectByIDFromDB(postID string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 and (type='Note' or type='Archive') union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 and (type='Note' or type='Archive')) as x`

	rows, err := config.DB.Query(query, postID)
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

		actor, err = GetActorFromDB(actor.Id)
		if err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		nColl.Actor = &actor

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

func GetObjectFromDB(id string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 order by updated desc`

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

func GetObjectFromDBCatalog(id string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 165`

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

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var replies CollectionBase

		post.Replies = &replies

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

func GetObjectFromDBFromID(id string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id like $1 and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id like $1 and type='Note') as x order by x.updated`

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(id)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		id = re.ReplaceAllString(id, "")
		id = "%" + match[1] + "/" + id
	}

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

func GetObjectFromDBPage(id string, page int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select count (x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit 15 offset $2`

	rows, err := config.DB.Query(query, id, page*15)
	if err != nil {
		return nColl, err
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		if err := rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int
		var err error
		post.Replies, postCnt, imgCnt, err = GetObjectRepliesDBLimit(post, 5)
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

	nColl.TotalItems = count
	nColl.OrderedItems = result

	return nColl, nil
}

func GetObjectFromJson(obj []byte) (ObjectBase, error) {
	var generic interface{}
	var nObj ObjectBase

	if err := json.Unmarshal(obj, &generic); err != nil {
		return ObjectBase{}, err
	}

	if generic != nil {
		switch generic.(type) {
		case []interface{}:
			var lObj ObjectBase
			var arrContext ObjectArray

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			if len(arrContext.Object) > 0 {
				lObj = arrContext.Object[0]
			}
			nObj = lObj
			break

		case map[string]interface{}:
			var arrContext Object

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			nObj = *arrContext.Object
			break

		case string:
			var lObj ObjectBase
			var arrContext ObjectString

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, err
			}

			lObj.Id = arrContext.Object
			nObj = lObj
			break
		}
	}

	return nObj, nil
}

func GetObjectFromPath(path string) (ObjectBase, error) {
	var nObj ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`

	rows, err := config.DB.Query(query, path)
	if err != nil {
		return nObj, err
	}

	defer rows.Close()
	rows.Next()
	var attachID string
	var previewID string

	var nActor Actor
	nObj.Actor = nActor.Id

	if err := rows.Scan(&nObj.Id, &nObj.Name, &nObj.Content, &nObj.Type, &nObj.Published, &nObj.AttributedTo, &attachID, &previewID, &nObj.Actor); err != nil {
		return nObj, err
	}

	var postCnt int
	var imgCnt int

	nObj.Replies, postCnt, imgCnt, err = GetObjectRepliesDB(nObj)
	if err != nil {
		return nObj, err
	}

	nObj.Replies.TotalItems, nObj.Replies.TotalImgs, err = GetObjectRepliesCount(nObj)
	if err != nil {
		return nObj, err
	}

	nObj.Replies.TotalItems = nObj.Replies.TotalItems + postCnt
	nObj.Replies.TotalImgs = nObj.Replies.TotalImgs + imgCnt

	nObj.Attachment, err = GetObjectAttachment(attachID)
	if err != nil {
		return nObj, err
	}

	nObj.Preview, err = GetObjectPreview(previewID)
	return nObj, err
}

func GetObjectImgsTotalDB(actor Actor) (int, error) {
	count := 0
	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`

	rows, err := config.DB.Query(query, actor.Id)
	if err != nil {
		return count, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func GetObjectPostsTotalDB(actor Actor) (int, error) {
	count := 0
	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`

	rows, err := config.DB.Query(query, actor.Id)
	if err != nil {
		return count, err
	}

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func GetObjectPreview(id string) (*NestedObjectBase, error) {
	var preview NestedObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published); err != nil {
			return nil, err
		}
	}

	return &preview, nil
}

func GetObjectRepliesCount(parent ObjectBase) (int, int, error) {
	var countId int
	var countImg int

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over() from (select id, attachment from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select id, attachment from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x`

	rows, err := config.DB.Query(query, parent.Id)
	if err != nil {
		return 0, 0, err
	}

	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&countId, &countImg)
	}

	return countId, countImg, err
}

func GetObjectRepliesDB(parent ObjectBase) (*CollectionBase, int, int, error) {
	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`

	rows, err := config.DB.Query(query, parent.Id)
	if err != nil {
		return nil, 0, 0, err
	}

	var postCount int
	var attachCount int

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt, err = GetObjectRepliesRepliesDB(post)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, postCount, attachCount, nil
}

func GetObjectRepliesDBLimit(parent ObjectBase, limit int) (*CollectionBase, int, int, error) {
	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published desc limit $2`

	rows, err := config.DB.Query(query, parent.Id, limit)
	if err != nil {
		return nil, 0, 0, err
	}

	var postCount int
	var attachCount int

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		var postCnt int
		var imgCnt int

		post.Replies, postCnt, imgCnt, err = GetObjectRepliesRepliesDB(post)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Replies.TotalItems = postCnt
		post.Replies.TotalImgs = imgCnt

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	sort.Sort(ObjectBaseSortAsc(nColl.OrderedItems))

	return &nColl, postCount, attachCount, nil
}

func GetObjectRepliesReplies(parent ObjectBase) (*CollectionBase, int, int, error) {
	var nColl CollectionBase
	var result []ObjectBase

	query := `select id, name, content, type, published, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') order by updated asc`

	rows, err := config.DB.Query(query, parent.Id)
	if err != nil {
		return &nColl, 0, 0, err
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, 0, 0, err
		}

		post.Actor = actor.Id

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, 0, 0, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, 0, 0, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, 0, 0, nil
}

func GetObjectRepliesRepliesDB(parent ObjectBase) (*CollectionBase, int, int, error) {
	var nColl CollectionBase
	var result []ObjectBase

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published asc`

	rows, err := config.DB.Query(query, parent.Id)
	if err != nil {
		return &nColl, 0, 0, err
	}

	var postCount int
	var attachCount int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor
		var attachID string
		var previewID string

		post.InReplyTo = append(post.InReplyTo, parent)

		if err := rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attachID, &previewID, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Actor = actor.Id

		post.Attachment, err = GetObjectAttachment(attachID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		post.Preview, err = GetObjectPreview(previewID)
		if err != nil {
			return &nColl, postCount, attachCount, err
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return &nColl, postCount, attachCount, nil
}

func GetObjectTypeDB(id string) (string, error) {
	query := `select type from activitystream where id=$1 union select type from cacheactivitystream where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var nType string
	rows.Next()
	rows.Scan(&nType)

	return nType, nil
}

func GetObjectsWithoutPreviewsCallback(callback func(id string, href string, mediatype string, name string, size int, published time.Time) error) error {
	query := `select id, href, mediatype, name, size, published from activitystream where id in (select attachment from activitystream where attachment!='' and preview='')`

	rows, err := config.DB.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var href string
		var mediatype string
		var name string
		var size int
		var published time.Time

		if err := rows.Scan(&id, &href, &mediatype, &name, &size, &published); err != nil {
			return err
		}

		if err := callback(id, href, mediatype, name, size, published); err != nil {
			return err
		}
	}

	return nil
}

func GetToFromJson(to []byte) ([]string, error) {
	var generic interface{}

	err := json.Unmarshal(to, &generic)
	if err != nil {
		return nil, err
	}

	if generic != nil {
		var nStr []string
		switch generic.(type) {
		case []interface{}:
			err = json.Unmarshal(to, &nStr)
			break
		case string:
			var str string
			err = json.Unmarshal(to, &str)
			nStr = append(nStr, str)
			break
		}
		return nStr, err
	}

	return nil, nil
}

func IsObjectCached(id string) (bool, error) {
	query := `select id from cacheactivitystream where id=$1`
	rows, err := config.DB.Query(query, id)
	if err != nil {
		return false, err
	}

	var nID string
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&nID)
	return nID != "", err
}

func IsIDLocal(id string) (bool, error) {
	activity, err := GetActivityFromDB(id)
	return len(activity.OrderedItems) > 0, err
}

func IsObjectLocal(id string) (bool, error) {
	query := `select id from activitystream where id=$1`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return false, err
	}

	var nID string
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&nID)
	return nID != "", err
}

func MarkObjectSensitive(id string, sensitive bool) error {
	var query = `update activitystream set sensitive=$1 where id=$2`
	if _, err := config.DB.Exec(query, sensitive, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set sensitive=$1 where id=$2`
	_, err := config.DB.Exec(query, sensitive, id)
	return err
}

func ObjectFromJson(r *http.Request, obj ObjectBase) (ObjectBase, error) {
	body, _ := ioutil.ReadAll(r.Body)

	var respActivity ActivityRaw

	err := json.Unmarshal(body, &respActivity)
	if err != nil {
		return obj, err
	}

	res, err := HasContextFromJson(respActivity.AtContextRaw.Context)

	if err == nil && res {
		var jObj ObjectBase
		jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
		if err != nil {
			return obj, err
		}

		jObj.To, err = GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return obj, err
		}

		jObj.Cc, err = GetToFromJson(respActivity.CcRaw)
	}

	return obj, err
}

func SetAttachmentFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id=$3)`

	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream  where id=$3)`

	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetAttachmentRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id in (select id from replies where inreplyto=$3))`

	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream where id in (select id from replies where inreplyto=$3))`

	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetPreviewFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id=$3)`

	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetPreviewRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id in (select id from replies where inreplyto=$3))`

	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id in (select id from replies where inreplyto=$3))`

	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetObject(id string, _type string) error {
	if err := SetAttachmentFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewFromDB(id, _type); err != nil {
		return err
	}

	return SetObjectFromDB(id, _type)
}

func SetObjectAndReplies(id string, _type string) error {
	if err := SetAttachmentFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewFromDB(id, _type); err != nil {
		return err
	}

	if err := SetObjectRepliesFromDB(id, _type); err != nil {
		return err
	}

	if err := SetAttachmentRepliesFromDB(id, _type); err != nil {
		return err
	}

	if err := SetPreviewRepliesFromDB(id, _type); err != nil {
		return err
	}

	return SetObjectFromDB(id, _type)
}

func SetObjectFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id=$3`

	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id=$3`

	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetObjectRepliesFromDB(id string, _type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err := config.DB.Exec(query, _type, datetime, id)
	return err
}

func SetObjectType(id string, nType string) error {
	col, err := GetObjectFromDB(id)
	if err != nil {
		return err
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := UpdateObjectTypeDB(k.Id, nType); err != nil {
				return err
			}
		}

		if err := UpdateObjectTypeDB(e.Id, nType); err != nil {
			return err
		}
	}

	return nil
}

func TombstoneAttachmentFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from activitystream where id=$3)`

	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from cacheactivitystream where id=$3)`

	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, id)
	return err
}

func TombstoneAttachmentRepliesFromDB(id string) error {
	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment string

		if err := rows.Scan(&attachment); err != nil {
			return err
		}

		if err := DeleteAttachmentFromFile(attachment); err != nil {
			return err
		}

		if err := TombstoneAttachmentFromDB(attachment); err != nil {
			return err
		}
	}

	return nil
}

func TombstonePreviewFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from activitystream where id=$3)`

	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`

	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, id)
	return err
}

func TombstonePreviewRepliesFromDB(id string) error {
	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`

	rows, err := config.DB.Query(query, id)
	if err != nil {
		return err
	}

	defer rows.Close()
	for rows.Next() {
		var attachment string

		if err := rows.Scan(&attachment); err != nil {
			return err
		}

		if err := DeletePreviewFromFile(attachment); err != nil {
			return err
		}

		if err := TombstonePreviewFromDB(attachment); err != nil {
			return err
		}
	}

	return nil
}

func TombstoneObject(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := TombstonePreviewFromDB(id); err != nil {
		return err
	}

	return TombstoneObjectFromDB(id)
}

func TombstoneObjectAndReplies(id string) error {
	if err := DeleteReportActivity(id); err != nil {
		return err
	}

	if err := DeleteAttachmentFromFile(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentFromDB(id); err != nil {
		return err
	}

	if err := DeletePreviewFromFile(id); err != nil {
		return err
	}

	if err := TombstonePreviewFromDB(id); err != nil {
		return err
	}

	if err := TombstoneObjectRepliesFromDB(id); err != nil {
		return err
	}

	if err := TombstoneAttachmentRepliesFromDB(id); err != nil {
		return err
	}

	if err := TombstonePreviewRepliesFromDB(id); err != nil {
		return err
	}

	return TombstoneObjectFromDB(id)
}

func TombstoneObjectFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)
	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id=$2`

	if _, err := config.DB.Exec(query, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='',  deleted=$1 where id=$2`

	_, err := config.DB.Exec(query, datetime, id)
	return err
}

func TombstoneObjectRepliesFromDB(id string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	if _, err := config.DB.Exec(query, datetime, id); err != nil {
		return err
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`

	_, err := config.DB.Exec(query, datetime, id)
	return err
}

func UpdateObjectTypeDB(id string, nType string) error {
	query := `update activitystream set type=$2 where id=$1 and type !='Tombstone'`
	if _, err := config.DB.Exec(query, id, nType); err != nil {
		return err
	}

	query = `update cacheactivitystream set type=$2 where id=$1 and type !='Tombstone'`
	_, err := config.DB.Exec(query, id, nType)
	return err
}

func UpdateObjectWithPreview(id string, preview string) error {
	query := `update activitystream set preview=$1 where attachment=$2`

	_, err := config.DB.Exec(query, preview, id)
	return err
}

func AddFollower(id string, follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`

	_, err := config.DB.Exec(query, id, follower)
	return err
}

func WriteObjectReplyToDB(obj ObjectBase) error {
	for i, e := range obj.InReplyTo {
		if res, err := CheckIfObjectOP(obj.Id); err == nil && !res && i == 0 {
			nType, err := GetObjectTypeDB(e.Id)
			if err != nil {
				return err
			}

			if nType == "Archive" {
				if err := UpdateObjectTypeDB(obj.Id, "Archive"); err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}

		query := `select id from replies where id=$1 and inreplyto=$2`

		var id string
		if err := config.DB.QueryRow(query, obj.Id, e.Id).Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			_, err := config.DB.Exec(query, obj.Id, e.Id)
			if err != nil {
				return err
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
			if err := WriteObjectUpdatesToDB(e); err != nil {
				return err
			}
		}
	}

	if len(obj.InReplyTo) < 1 {
		query := `select id from replies where id=$1 and inreplyto=$2`

		var id string
		if err := config.DB.QueryRow(query, obj.Id, "").Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`

			if _, err := config.DB.Exec(query, obj.Id, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteObjectReplyToLocalDB(id string, replyto string) error {
	query := `select id from replies where id=$1 and inreplyto=$2`

	rows, err := config.DB.Query(query, id, replyto)
	if err != nil {
		return err
	}
	defer rows.Close()

	var nID string
	rows.Next()
	rows.Scan(&nID)

	if nID == "" {
		query := `insert into replies (id, inreplyto) values ($1, $2)`

		if _, err := config.DB.Exec(query, id, replyto); err != nil {
			return err
		}
	}

	query = `select inreplyto from replies where id=$1`

	rows, err = config.DB.Query(query, replyto)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var val string
		rows.Scan(&val)
		if val == "" {
			updated := time.Now().UTC().Format(time.RFC3339)
			query := `update activitystream set updated=$1 where id=$2`

			if _, err := config.DB.Exec(query, updated, replyto); err != nil {
				return err
			}
		}
	}

	return nil
}

func WriteObjectToCache(obj ObjectBase) (ObjectBase, error) {
	if res, err := util.IsPostBlacklist(obj.Content); err == nil && res {
		fmt.Println("\n\nBlacklist post blocked\n\n")
		return obj, nil
	} else {
		return obj, err
	}

	if len(obj.Attachment) > 0 {
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

	WriteObjectReplyToDB(obj)

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			WriteObjectToCache(e)
		}
	}

	return obj, nil
}

func WriteObjectToDB(obj ObjectBase) (ObjectBase, error) {
	id, err := util.CreateUniqueID(obj.Actor)
	if err != nil {
		return obj, err
	}

	obj.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, err
			}

			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Preview.Published = time.Now().UTC()
			obj.Preview.Updated = time.Now().UTC()
			obj.Preview.AttributedTo = obj.Id
			if err := WritePreviewToDB(*obj.Preview); err != nil {
				return obj, err
			}
		}
		for i := range obj.Attachment {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, err
			}

			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Attachment[i].Published = time.Now().UTC()
			obj.Attachment[i].Updated = time.Now().UTC()
			obj.Attachment[i].AttributedTo = obj.Id
			WriteAttachmentToDB(obj.Attachment[i])
			WriteActivitytoDBWithAttachment(obj, obj.Attachment[i], *obj.Preview)
		}

	} else {
		if err := WriteActivitytoDB(obj); err != nil {
			return obj, err
		}
	}

	if err := WriteObjectReplyToDB(obj); err != nil {
		return obj, err
	}

	err = WriteWalletToDB(obj)
	return obj, err
}

func WriteObjectUpdatesToDB(obj ObjectBase) error {
	query := `update activitystream set updated=$1 where id=$2`

	if _, err := config.DB.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id); err != nil {
		return err
	}

	query = `update cacheactivitystream set updated=$1 where id=$2`

	_, err := config.DB.Exec(query, time.Now().UTC().Format(time.RFC3339), obj.Id)
	return err
}

func WriteWalletToDB(obj ObjectBase) error {
	for _, e := range obj.Option {
		if e == "wallet" {
			for _, e := range obj.Wallet {
				query := `insert into wallet (id, type, address) values ($1, $2, $3)`

				if _, err := config.DB.Exec(query, obj.Id, e.Type, e.Address); err != nil {
					return err
				}
			}

			return nil
		}
	}
	return nil
}
