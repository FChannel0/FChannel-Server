package db

import (
	"database/sql"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

type NewsItem struct {
	Title   string
	Content template.HTML
	Time    int
}

// ConnectDB connects to the PostgreSQL database configured.
func ConnectDB() error {
	host := config.DBHost
	port := config.DBPort
	user := config.DBUser
	password := config.DBPassword
	dbname := config.DBName

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	_db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return err
	}

	if err := _db.Ping(); err != nil {
		return err
	}

	fmt.Println("Successfully connected DB")

	config.DB = _db
	return nil
}

// Close closes the database connection.
func Close() error {
	return config.DB.Close()
}

func RunDatabaseSchema() error {
	query, err := ioutil.ReadFile("databaseschema.psql")
	if err != nil {
		return err
	}

	_, err = config.DB.Exec(string(query))
	return err
}

func CreateNewBoardDB(actor activitypub.Actor) (activitypub.Actor, error) {
	query := `insert into actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary, restricted) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := config.DB.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted)

	if err != nil {
		// TODO: board exists error
		return activitypub.Actor{}, err
	} else {
		fmt.Println("board added")

		for _, e := range actor.AuthRequirement {
			query = `insert into actorauth (type, board) values ($1, $2)`

			if _, err := config.DB.Exec(query, e, actor.Name); err != nil {
				return activitypub.Actor{}, err
			}
		}

		var verify Verify

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "admin"

		CreateVerification(verify)

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "janitor"

		CreateVerification(verify)

		verify.Identifier = actor.Id
		verify.Code = util.CreateKey(50)
		verify.Type = "post"

		CreateVerification(verify)

		var nverify Verify
		nverify.Board = actor.Id
		nverify.Identifier = "admin"
		nverify.Type = "admin"
		CreateBoardMod(nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "janitor"
		nverify.Type = "janitor"
		CreateBoardMod(nverify)

		nverify.Board = actor.Id
		nverify.Identifier = "post"
		nverify.Type = "post"
		CreateBoardMod(nverify)

		activitypub.CreatePem(actor)

		if actor.Name != "main" {
			var nObject activitypub.ObjectBase
			var nActivity activitypub.Activity

			nActor, err := activitypub.GetActorFromDB(config.Domain)
			if err != nil {
				return actor, err
			}

			nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			nActivity.Type = "Follow"
			nActivity.Actor = &nActor
			nActivity.Object = &nObject

			mActor, err := activitypub.GetActorFromDB(actor.Id)
			if err != nil {
				return actor, err
			}

			nActivity.Object.Actor = mActor.Id
			nActivity.To = append(nActivity.To, actor.Id)

			response := AcceptFollow(nActivity)
			if _, err := SetActorFollowingDB(response); err != nil {
				return actor, err
			}
			if err := MakeActivityRequest(nActivity); err != nil {
				return actor, err
			}
		}

	}

	return actor, nil
}

func RemovePreviewFromFile(id string) error {
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

	return activitypub.DeletePreviewFromDB(id)
}

func GetRandomCaptcha() (string, error) {
	var verify string

	query := `select identifier from verification where type='captcha' order by random() limit 1`

	rows, err := config.DB.Query(query)
	if err != nil {
		return verify, err
	}
	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&verify); err != nil {
		return verify, err
	}

	return verify, nil
}

func GetCaptchaTotal() (int, error) {
	query := `select count(*) from verification where type='captcha'`

	rows, err := config.DB.Query(query)
	if err != nil {
		return 0, err
	}

	defer rows.Close()

	var count int
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return count, err
		}
	}

	return count, nil
}

func GetCaptchaCodeDB(verify string) (string, error) {
	query := `select code from verification where identifier=$1 limit 1`

	rows, err := config.DB.Query(query, verify)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var code string

	rows.Next()
	if err := rows.Scan(&code); err != nil {
		fmt.Println("Could not get verification captcha")
	}

	return code, nil
}

func DeleteCaptchaCodeDB(verify string) error {
	query := `delete from verification where identifier=$1`

	_, err := config.DB.Exec(query, verify)
	if err != nil {
		return err
	}

	return os.Remove("./" + verify)
}

//if limit less than 1 return all news items
func GetNewsFromDB(limit int) ([]NewsItem, error) {
	var news []NewsItem

	var query string
	if limit > 0 {
		query = `select title, content, time from newsItem order by time desc limit $1`
	} else {
		query = `select title, content, time from newsItem order by time desc`
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = config.DB.Query(query, limit)
	} else {
		rows, err = config.DB.Query(query)
	}

	if err != nil {
		return news, nil
	}

	defer rows.Close()
	for rows.Next() {
		n := NewsItem{}
		var content string
		if err := rows.Scan(&n.Title, &content, &n.Time); err != nil {
			return news, err
		}

		content = strings.ReplaceAll(content, "\n", "<br>")
		n.Content = template.HTML(content)

		news = append(news, n)
	}

	return news, nil
}

func GetNewsItemFromDB(timestamp int) (NewsItem, error) {
	var news NewsItem
	var content string
	query := `select title, content, time from newsItem where time=$1 limit 1`

	rows, err := config.DB.Query(query, timestamp)
	if err != nil {
		return news, err
	}

	defer rows.Close()

	rows.Next()
	if err := rows.Scan(&news.Title, &content, &news.Time); err != nil {
		return news, err
	}

	content = strings.ReplaceAll(content, "\n", "<br>")
	news.Content = template.HTML(content)

	return news, nil
}

func deleteNewsItemFromDB(timestamp int) error {
	query := `delete from newsItem where time=$1`
	_, err := config.DB.Exec(query, timestamp)
	return err
}

func WriteNewsToDB(news NewsItem) error {
	query := `insert into newsItem (title, content, time) values ($1, $2, $3)`

	_, err := config.DB.Exec(query, news.Title, news.Content, time.Now().Unix())
	return err
}

func AddInstanceToInactiveDB(instance string) error {
	query := `select timestamp from inactive where instance=$1`

	rows, err := config.DB.Query(query, instance)
	if err != nil {
		return err
	}

	var timeStamp string
	defer rows.Close()
	rows.Next()
	rows.Scan(&timeStamp)

	if timeStamp == "" {
		query := `insert into inactive (instance, timestamp) values ($1, $2)`

		_, err := config.DB.Exec(query, instance, time.Now().UTC().Format(time.RFC3339))
		return err
	}

	if !IsInactiveTimestamp(timeStamp) {
		return nil
	}

	query = `delete from following where following like $1`
	if _, err := config.DB.Exec(query, "%"+instance+"%"); err != nil {
		return err
	}

	query = `delete from follower where follower like $1`
	if _, err = config.DB.Exec(query, "%"+instance+"%"); err != nil {
		return err
	}

	return DeleteInstanceFromInactiveDB(instance)
}

func DeleteInstanceFromInactiveDB(instance string) error {
	query := `delete from inactive where instance=$1`

	_, err := config.DB.Exec(query, instance)
	return err
}

func IsInactiveTimestamp(timeStamp string) bool {
	stamp, _ := time.Parse(time.RFC3339, timeStamp)
	if time.Now().UTC().Sub(stamp).Hours() > 48 {
		return true
	}

	return false
}

func ArchivePosts(actor activitypub.Actor) error {
	if actor.Id != "" && actor.Id != config.Domain {
		col, err := activitypub.GetAllActorArchiveDB(actor.Id, 165)
		if err != nil {
			return err
		}

		for _, e := range col.OrderedItems {
			for _, k := range e.Replies.OrderedItems {
				if err := activitypub.UpdateObjectTypeDB(k.Id, "Archive"); err != nil {
					return err
				}
			}

			if err := activitypub.UpdateObjectTypeDB(e.Id, "Archive"); err != nil {
				return err
			}
		}
	}

	return nil
}

func UnArchiveLast(actorId string) error {
	col, err := activitypub.GetActorCollectionDBTypeLimit(actorId, "Archive", 1)
	if err != nil {
		return err
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := activitypub.UpdateObjectTypeDB(k.Id, "Note"); err != nil {
				return err
			}
		}

		if err := activitypub.UpdateObjectTypeDB(e.Id, "Note"); err != nil {
			return err
		}
	}

	return nil
}

func IsReplyInThread(inReplyTo string, id string) (bool, error) {
	obj, _, err := webfinger.CheckValidActivity(inReplyTo)
	if err != nil {
		return false, err
	}

	for _, e := range obj.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true, nil
		}
	}

	return false, nil
}

func IsReplyToOP(op string, link string) (string, bool, error) {
	if op == link {
		return link, true, nil
	}

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(link)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		link = re.ReplaceAllString(link, "")
		link = "%" + match[1] + "/" + link
	}

	query := `select id from replies where id like $1 and inreplyto=$2`

	rows, err := config.DB.Query(query, link, op)
	if err != nil {
		return op, false, err
	}

	defer rows.Close()

	var id string
	rows.Next()
	if err := rows.Scan(&id); err != nil {
		return id, false, err
	}

	return id, id != "", nil
}

func GetReplyOP(link string) (string, error) {
	query := `select id from replies where id in (select inreplyto from replies where id=$1) and inreplyto=''`

	rows, err := config.DB.Query(query, link)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var id string

	rows.Next()
	err = rows.Scan(&id)
	return id, err
}

func StartupArchive() error {
	for _, e := range webfinger.FollowingBoards {
		actor, err := activitypub.GetActorFromDB(e.Id)
		if err != nil {
			return err
		}

		if err := ArchivePosts(actor); err != nil {
			return err
		}
	}

	return nil
}

func CheckInactive() {
	for true {
		CheckInactiveInstances()
		time.Sleep(24 * time.Hour)
	}
}

func CheckInactiveInstances() (map[string]string, error) {
	instances := make(map[string]string)
	query := `select following from following`

	rows, err := config.DB.Query(query)
	if err != nil {
		return instances, err
	}
	defer rows.Close()

	for rows.Next() {
		var instance string
		if err := rows.Scan(&instance); err != nil {
			return instances, err
		}

		instances[instance] = instance
	}

	query = `select follower from follower`
	rows, err = config.DB.Query(query)
	if err != nil {
		return instances, err
	}
	defer rows.Close()

	for rows.Next() {
		var instance string
		if err := rows.Scan(&instance); err != nil {
			return instances, err
		}

		instances[instance] = instance
	}

	re := regexp.MustCompile(config.Domain + `(.+)?`)
	for _, e := range instances {
		actor, err := webfinger.GetActor(e)
		if err != nil {
			return instances, err
		}

		if actor.Id == "" && !re.MatchString(e) {
			if err := AddInstanceToInactiveDB(e); err != nil {
				return instances, err
			}
		} else {
			if err := DeleteInstanceFromInactiveDB(e); err != nil {
				return instances, err
			}
		}
	}

	return instances, nil
}

func GetAdminAuth() (string, string, error) {
	query := fmt.Sprintf("select identifier, code from boardaccess where board='%s' and type='admin'", config.Domain)

	rows, err := config.DB.Query(query)
	if err != nil {
		return "", "", err
	}

	var code string
	var identifier string

	rows.Next()
	err = rows.Scan(&identifier, &code)

	return code, identifier, err
}

func IsHashBanned(hash string) (bool, error) {
	var h string

	query := `select hash from bannedmedia where hash=$1`

	rows, err := config.DB.Query(query, hash)
	if err != nil {
		return true, err
	}
	defer rows.Close()

	rows.Next()
	err = rows.Scan(&h)

	return h == hash, err
}
