package main

import "fmt"
import "strings"
import "strconv"
import "net/http"
import "net/url"
import "database/sql"
import _ "github.com/lib/pq"
import "math/rand"
import "time"
import "regexp"
import "os/exec"
import "bytes"
import "encoding/json"
import "io/ioutil"
import "mime/multipart"
import "os"
import "bufio"

var Port = ":" + GetConfigValue("instanceport")
var TP   = GetConfigValue("instancetp")
var Domain = TP + "" + GetConfigValue("instance")

var authReq = []string{"captcha","email","passphrase"}

var supportedFiles = []string{"image/gif","image/jpeg","image/png","image/svg+xml","image/webp","image/avif","image/apng","video/mp4","video/ogg","video/webm","audio/mpeg","audio/ogg","audio/wav", "audio/wave", "audio/x-wav"}

var SiteEmail = GetConfigValue("emailaddress")        //contact@fchan.xyz
var SiteEmailPassword = GetConfigValue("emailpass")
var SiteEmailServer = GetConfigValue("emailserver")   //mail.fchan.xyz
var SiteEmailPort = GetConfigValue("emailport")       //587

type BoardAccess struct {
	boards []string
}

func main() {

	if _, err := os.Stat("./public"); os.IsNotExist(err) {
    os.Mkdir("./public", 0755)
	}	

	db := ConnectDB();

	defer db.Close()

	go MakeCaptchas(db, 100)
	
	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if GetConfigValue("instancename") != "" {
		CreateNewBoardDB(db, *CreateNewActor("", GetConfigValue("instancename"), GetConfigValue("instancesummary"), authReq, false))
	}
	
	// Allow access to public media folder
	fileServer := http.FileServer(http.Dir("./public"))
	http.Handle("/public/", http.StripPrefix("/public", neuter(fileServer)))		

	// main routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
		path := r.URL.Path

		// remove trailing slash
		if path != "/" {
			re := regexp.MustCompile(`/$`)
			path = re.ReplaceAllString(path, "")
		}

		method := r.Method

		actor := GetActorFromPath(db, path, "/")

		var mainActor bool
		var mainInbox bool
		var mainOutbox bool
		var mainFollowing bool
		var mainFollowers bool		

		var actorMain bool
		var actorInbox bool
		var actorOutbox bool
		var actorFollowing bool
		var actorFollowers bool
		var actorReported bool		
		var actorVerification bool


		if(actor.Id != ""){
			if actor.Name == "main" {
				mainActor = (path == "/")			
				mainInbox = (path == "/inbox")
				mainOutbox = (path == "/outbox")
				mainFollowing = (path == "/following")
				mainFollowers = (path == "/followers")			
			} else {
				actorMain = (path == "/" + actor.Name)			
				actorInbox = (path == "/" + actor.Name + "/inbox")
				actorOutbox = (path == "/" + actor.Name + "/outbox")
				actorFollowing = (path == "/" + actor.Name + "/following")
				actorFollowers = (path == "/" + actor.Name + "/followers")
				actorReported = (path == "/" + actor.Name + "/reported")				
				actorVerification = (path == "/" + actor.Name + "/verification")						
			}
		}

		if mainActor {
			GetActorInfo(w, db, Domain)
		} else if mainInbox {
			if method == "POST" {
				
			} else {
				w.WriteHeader(http.StatusForbidden)				
				w.Write([]byte("404 no path"))
			}			
		} else if mainOutbox {
			if method == "GET" {
				GetActorOutbox(w, r, db)
			} else if method == "POST" {
				ParseOutboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)			
				w.Write([]byte("404 no path"))
			}
		} else if mainFollowing {
			GetActorFollowing(w, db, Domain)			
		} else if mainFollowers {
			GetActorFollowers(w, db, Domain)						
		} else if actorMain {
			GetActorInfo(w, db, actor.Id)			
		} else if actorInbox {
			if method == "POST" {
				ParseInboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("404 no path"))							
			}
		} else if actorOutbox {
			if method == "GET" {
				GetActorOutbox(w, r, db)				
			} else if method == "POST" {
				ParseOutboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("404 no path"))											
			}			
		} else if actorFollowing {
			GetActorFollowing(w, db, actor.Id)
		} else if actorFollowers {
			GetActorFollowers(w, db, actor.Id)
		} else if actorReported {
			GetActorReported(w, r, db, actor.Id)
		} else if  actorVerification {
			if method == "POST" {
				p, _ := url.ParseQuery(r.URL.RawQuery)
				if len(p["email"]) > 0 {
					email := p["email"][0]
					verify := GetVerificationByEmail(db, email)
					if verify.Identifier != "" || !IsEmailSetup() {
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte("400 no path"))																	
					} else {
						var nVerify Verify
						nVerify.Type = "email"						
						nVerify.Identifier = email
						nVerify.Code = CreateKey(32)
						nVerify.Board = actor.Id
						CreateVerification(db, nVerify)
						SendVerification(nVerify)
						w.WriteHeader(http.StatusCreated)
						w.Write([]byte("Verification added"))																							
					}

				} else {
					w.WriteHeader(http.StatusForbidden)
					w.Write([]byte("400 no path"))											
				}
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("400 no path"))											
			}
		} else {
			collection := GetCollectionFromPath(db, Domain + "" + path)
			if len(collection.OrderedItems) > 0 {
				enc, _ := json.MarshalIndent(collection, "", "\t")							
				w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
				w.Write(enc)				
			} else {
				w.WriteHeader(http.StatusForbidden)			
				w.Write([]byte("404 no path"))
			}
		}
	})

	http.HandleFunc("/getcaptcha", func(w http.ResponseWriter, r *http.Request){
		w.Write([]byte(GetRandomCaptcha(db)))
	})

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request){
		values := r.URL.Query().Get("id")

		header := r.Header.Get("Authorization")

		auth := strings.Split(header, " ")

		if len(values) < 1 || !IsIDLocal(db, values) || len(auth) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		actor := GetActorFromPath(db, values, "/")

		if !HasAuth(db, auth[1], actor.Id) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}		

		var obj ObjectBase
		obj.Id = values

		count, _ := GetObjectRepliesDBCount(db, obj)
		if count == 0 {
			DeleteObject(db, obj.Id)
		} else {
			DeleteObjectAndReplies(db, obj.Id)
		}
		w.Write([]byte(""))
	})

	http.HandleFunc("/deleteattach", func(w http.ResponseWriter, r *http.Request){
		
		values := r.URL.Query().Get("id")

		header := r.Header.Get("Authorization")

		auth := strings.Split(header, " ")

		if len(values) < 1 || !IsIDLocal(db, values) || len(auth) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		actor := GetActorFromPath(db, values, "/")

		if !HasAuth(db, auth[1], actor.Id) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		id := values
		DeleteAttachmentFromFile(db, id)
		DeletePreviewFromFile(db, id)		
		w.Write([]byte(""))		
	})

	http.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request){
		
		id := r.URL.Query().Get("id")
		close := r.URL.Query().Get("close")
		header := r.Header.Get("Authorization")

		auth := strings.Split(header, " ")		
		if close == "1" {
			if !IsIDLocal(db, id) || len(auth) < 2 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))					
				return				
			}

			actor := GetActorFromPath(db, id, "/")

			if !HasAuth(db, auth[1], actor.Id) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				return
			}

			reported := DeleteReportActivity(db, id)
			if reported {
				w.Write([]byte(""))			
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}
		
		reported := ReportActivity(db, id)

		if reported {
			w.Write([]byte(""))
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))					
	})

	http.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request){
		var verify Verify
		defer r.Body.Close()

		body, _ := ioutil.ReadAll(r.Body)

		err := json.Unmarshal(body, &verify)

		CheckError(err, "error get verify from json")

		v := GetVerificationByCode(db, verify.Code)

		if v.Identifier == verify.Identifier {
			w.Write([]byte(v.Board))
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))		
	})

	fmt.Println("Server for " + Domain + " running on port " + Port)
	
	PrintAdminAuth(db)
	
	http.ListenAndServe(Port, nil)	
}

func CheckError(e error, m string) error{
	if e != nil {
		fmt.Println()		
		fmt.Println(m)
		fmt.Println()		
		panic(e)
	}

	return e
}

func ConnectDB() *sql.DB {

	host     := GetConfigValue("dbhost")
	port,_   := strconv.Atoi(GetConfigValue("dbport"))
	user     := GetConfigValue("dbuser")
	password := GetConfigValue("dbpass")
	dbname   := GetConfigValue("dbname")
	
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s " +
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)
	CheckError(err, "error with db connection")

	err = db.Ping()

	CheckError(err, "error with db ping")

	fmt.Println("Successfully connected DB")
	return db
}

func CreateKey(len int) string {
	var key string
	str := (CreateTripCode(RandomID(len)))
	for i := 0; i < len; i++ {
		key += fmt.Sprintf("%c", str[i])			
	}
	return key
}

func neuter(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasSuffix(r.URL.Path, "/") {
            http.NotFound(w, r)
            return
        }

        next.ServeHTTP(w, r)
    })
}

func CreateTripCode(input string) string {
	cmd := exec.Command("sha512sum")
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	CheckError(err, "error with create trip code")

	return out.String()
}

func CreateNameTripCode(input string) string {
	re := regexp.MustCompile("#.+")
	chunck := re.FindString(input)
	hash := CreateTripCode(chunck)
	return re.ReplaceAllString(input, hash[0:8])
}

func GetActorFromPath(db *sql.DB, location string, prefix string) Actor {
	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)

	var actor string

	if(len(match) < 1 ) {
		actor = "/"
	} else {
		actor = strings.Replace(match[1], "/", "", -1)
	}

	if actor == "/" || actor == "outbox" || actor == "inbox" || actor == "following" || actor == "followers" {
		actor = Domain
	} else {
		actor = Domain + "/" + actor
	}

	var nActor Actor
	
	nActor = GetActorFromDB(db, actor)

	return nActor
}

func GetContentType(location string) string {
	elements := strings.Split(location, ";")
	if len(elements) > 0 {
		return elements[0]	
	} else {
		return location
	}
}

func RandomID(size int) string {
	rand.Seed(time.Now().UnixNano())
	domain := "0123456789ABCDEFG"
	rng := size
	newID := ""
	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}
	
	return newID
}

func CreateUniqueID(db *sql.DB, actor string) string {
	var newID string	
	isUnique := false
	for !isUnique {
		newID = RandomID(8)
		
		query := fmt.Sprintf("select id from activitystream where id='%s/%s/%s'", Domain, actor, newID)

		rows, err := db.Query(query)

		CheckError(err, "error with unique id query")

		defer rows.Close()

		var count int = 0
		for rows.Next(){
			count += 1
		}
		
		if count < 1 {
			isUnique = true
		}
	}
	
	return newID
}

func CreateNewActor(board string, prefName string, summary string, authReq []string, restricted bool) *Actor{
	actor := new(Actor)

	var path string
	if board == "" {
		path = Domain
		actor.Name = "main"
	} else {
		path = Domain + "/" + board
		actor.Name = board
	}

	actor.Type = "Service"
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

func GetActorInfo(w http.ResponseWriter, db *sql.DB, id string) {
	actor := GetActorFromDB(db, id)
	enc, _ := json.MarshalIndent(actor, "", "\t")
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().Format(time.RFC3339)
	nObj.Updated = time.Now().Format(time.RFC3339)

	return nObj
}

func CreateActivity(activityType string, obj ObjectBase) Activity {
	var newActivity Activity

	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = obj.Actor
	newActivity.Object = &obj

	for _, e := range obj.To {
		if obj.Actor.Id != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor.Id != e {		
			newActivity.Cc = append(newActivity.Cc, e)
		}
	}	

	return newActivity
}

func ProcessActivity(db *sql.DB, activity Activity) {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if GetActorFromDB(db, e).Id != "" {
				fmt.Println("actor is in the database")
			} else {
				fmt.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {

	} else if activityType == "Delete" {

	}
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

	href := GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)

	objFile := re.FindString(obj.Href)

	cmd := exec.Command("convert", "." + objFile ,"-resize", "250x250>", "." + href)

	err := cmd.Run()

	if CheckError(err, "error with resize attachment preview")	!= nil {
		var preview NestedObjectBase
		return &preview
	}

	return &nPreview
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]ObjectBase, *os.File) {
	contentType, _ := GetFileContentType(file)
	filename := header.Filename
	size := header.Size

	re := regexp.MustCompile(`.+/`)

	fileType := re.ReplaceAllString(contentType, "")

	tempFile, _ := ioutil.TempFile("./public", "*." + fileType)

	var nAttachment []ObjectBase
	var image ObjectBase
	
	image.Type = "Attachment"
	image.Name = filename
	image.Href = Domain + "/" + tempFile.Name()
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().Format(time.RFC3339)

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile
}

func ParseCommentForReplies(comment string) []ObjectBase {
	
	re := regexp.MustCompile("(>>)(https://|http://)?(www\\.)?.+\\/\\w+")
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i:= 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)		
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)		
		str = TP + "" + str
		if !IsInStringArray(links, str) {
			links = append(links, str)
		}
	}

	var validLinks []ObjectBase
	for i:= 0; i < len(links); i++ {
		_, isValid := CheckValidActivity(links[i])
		if(isValid) {
			var reply = new(ObjectBase)
			reply.Id = links[i]
			reply.Published = time.Now().Format(time.RFC3339)
			validLinks = append(validLinks, *reply)
		}
	}

	return validLinks
}


func CheckValidActivity(id string) (Collection, bool) {

	req, err := http.NewRequest("GET", id, nil)

	if err != nil {
		fmt.Println("error with request")
		panic(err)
	}

	req.Header.Set("Accept", "json/application/activity+json")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Println("error with response")
		panic(err)		
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respCollection Collection

	err = json.Unmarshal(body, &respCollection)

	if err != nil {
		panic(err)
	}

	if respCollection.AtContext.Context == "https://www.w3.org/ns/activitystreams" &&  respCollection.OrderedItems[0].Id != "" {
		return respCollection, true;
	}

	return respCollection, false;
}

func GetActor(id string) Actor {

	var respActor Actor

	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "error with getting actor req")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Println("error with getting actor resp")
		return respActor
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &respActor)

	CheckError(err, "error getting actor from body")

	return respActor
}

func GetActorCollection(collection string) Collection {
	var nCollection Collection

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req " + collection)

	resp, err := http.DefaultClient.Do(req)

	CheckError(err, "error with getting actor collection resp " + collection)

	if resp.StatusCode == 200 {

		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		err = json.Unmarshal(body, &nCollection)

		CheckError(err, "error getting actor collection from body " + collection)
	}
	
	return nCollection
}

func IsValidActor(id string) (Actor, bool) {
	var respCollection Actor	
	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "error with valid actor request")

	req.Header.Set("Accept", "json/application/activity+json")

	resp, err := http.DefaultClient.Do(req)

	CheckError(err, "error with valid actor response")

	if resp.StatusCode == 403 {
		return respCollection, false;	
	}
	
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, &respCollection)

	if err != nil {
		panic(err)
	}

	if respCollection.AtContext.Context == "https://www.w3.org/ns/activitystreams" &&  respCollection.Id != "" && respCollection.Inbox != "" && respCollection.Outbox != "" {
		return respCollection, true;
	}

	return respCollection, false;	
}



func IsActivityLocal(db *sql.DB, activity Activity) bool {
	for _, e := range activity.To {
		if GetActorFromDB(db, e).Id != "" {
			return true
		}
	}

	for _, e := range activity.Cc {
		if GetActorFromDB(db, e).Id != "" {
			return true
		}
	}

	if activity.Actor != nil && GetActorFromDB(db, activity.Actor.Id).Id != "" {
		return true
	}

	return false
}

func IsIDLocal(db *sql.DB, id string) bool {

	if GetActivityFromDB(db, id).OrderedItems != nil {
		return true
	}
	
	return false
}

func IsObjectLocal(db *sql.DB, id string) bool {

	query := fmt.Sprintf("select id from activitystream where id='%s'", id)

	rows, err := db.Query(query)

	defer rows.Close()
	
	if err != nil {
		return false
	}

	return true
}

func GetObjectFromActivity(activity Activity) ObjectBase {
	return *activity.Object 
}

func MakeCaptchas(db *sql.DB, total int) {
	difference := total - GetCaptchaTotal(db)
	
	for i := 0; i < difference; i++ {
		CreateNewCaptcha(db)
	}
}

func GetFileContentType(out multipart.File) (string, error) {

	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	out.Seek(0, 0)

	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func IsReplyInThread(db *sql.DB, inReplyTo string, id string) bool {
	obj, _ := CheckValidActivity(inReplyTo)

	for _, e := range obj.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true
		}
	}
	return false
}

func SupportedMIMEType(mime string) bool {
	for _, e := range supportedFiles {
		if e == mime {
			return true
		}
	}
	
	return false
}	

func DeleteReportActivity(db *sql.DB, id string) bool {

	query := fmt.Sprintf("delete from reported where id='%s'", id)

	_, err := db.Exec(query)

	if err != nil {
		CheckError(err, "error closing reported activity")
		return false
	}
	
	return true
}

func ReportActivity(db *sql.DB, id string) bool {

	if !IsIDLocal(db, id) {
		return false
	}

	actor := GetActivityFromDB(db, id)
	
	query := fmt.Sprintf("select count from reported where id='%s'", id)

	rows, err := db.Query(query)

	CheckError(err, "could not select count from reported")

	defer rows.Close()
	var count int
	for rows.Next() {
		rows.Scan(&count)
	}

	if count < 1 {
		query = fmt.Sprintf("insert into reported (id, count, board) values ('%s', %d, '%s')", id, 1, actor.Actor.Id)

		_, err := db.Exec(query)

		if err != nil {
			CheckError(err, "error inserting new reported activity")
			return false
		}
		
	} else {
		count = count + 1
		query = fmt.Sprintf("update reported set count=%d where id='%s'", count, id)

		_, err := db.Exec(query)
		
		if err != nil {
			CheckError(err, "error updating reported activity")
			return false
		}
	}

	return true
}

func GetActorReported(w http.ResponseWriter, r *http.Request, db *sql.DB, id string) {

	auth := r.Header.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}

	if !HasAuth(db, verification[1], id) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))
		return
	}
	
	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems = GetActorReportedTotal(db, id)
	following.Items = GetActorReportedDB(db, id)

	enc, _ := json.MarshalIndent(following, "", "\t")							
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func MakeActivityRequest(activity Activity) {

	j, _ := json.MarshalIndent(activity, "", "\t")
	
	for _, e := range activity.To {
		
		actor := GetActor(e)

		if actor.Inbox != "" {
		req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))

		CheckError(err, "error with sending activity req to")

		_, err = http.DefaultClient.Do(req)

			CheckError(err, "error with sending activity resp to")
		}
	}	
}

func GetCollectionFromID(id string) Collection {
	var nColl Collection
	
	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "could not get collection from id req")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Println("could not get collection from " + id)
		return nColl
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()
		
		body, _ := ioutil.ReadAll(resp.Body)

		err = json.Unmarshal(body, &nColl)

		CheckError(err, "error getting collection resp from json body")

	}

	return nColl
}

func GetActorFromID(id string) Actor {
	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "error getting actor from id req")

	resp, err := http.DefaultClient.Do(req)

	CheckError(err, "error getting actor from id resp")

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respCollection Collection

	err = json.Unmarshal(body, &respCollection)

	CheckError(err, "error getting actor resp from json body")

	return *respCollection.OrderedItems[0].Actor
}

func GetConfigValue(value string) string{
	file, err := os.Open("config")

	CheckError(err, "there was an error opening the config file")

	defer file.Close()

	lines := bufio.NewScanner(file)

	for lines.Scan() {
		line := strings.SplitN(lines.Text(), ":", 2)
		if line[0] == value {
			return line[1]
		}
	}

	return ""
}


func PrintAdminAuth(db *sql.DB){
	query := fmt.Sprintf("select identifier, code from boardaccess where board='%s' and type='admin'", Domain)

	rows, err := db.Query(query)

	CheckError(err, "Error getting Domain auth")

	var code string
	var identifier string
	
	rows.Next()
	rows.Scan(&identifier, &code)

	fmt.Println("Admin Login: " + identifier + ", Code: " + code) 
}

func IsInStringArray(array []string, value string) bool {
	for _, e := range array {
		if e == value {
			return true
		}
	}
	return false
}


func GetUniqueFilename(_type string) string {
	id   := RandomID(8)
	file := "/public/" + id + "." + _type
	
	for true {
		if _, err := os.Stat("." + file); err == nil {
			id   = RandomID(8)			
			file = "/public/" + id + "." + _type
		}else{
			return "/public/" + id + "." + _type
		}
	}

	return ""
}

func DeleteObjectRequest(db *sql.DB, id string) {
	var nObj ObjectBase
	nObj.Id = id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectFromPath(db, id)
	followers := GetActorFollowDB(db, obj.Actor.Id)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(db, obj.Actor.Id)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}	

	MakeActivityRequest(activity)
}

func DeleteObjectAndRepliesRequest(db *sql.DB, id string) {
	var nObj ObjectBase
	nObj.Id = id
	
	activity := CreateActivity("Delete", nObj)
	
	obj := GetObjectFromPath(db, id)

	followers := GetActorFollowDB(db, obj.Actor.Id)	
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(activity)
}

func ResizeAttachmentToPreview(db *sql.DB) {
	query := `select id, href, mediatype, name, size, published from activitystream where id in (select attachment from activitystream where attachment!='' and preview='')`

	rows, err := db.Query(query)

	CheckError(err, "error getting attachments")


	defer rows.Close()
	for rows.Next() {

		var id string		
		var href string
		var mediatype string
		var name string
		var size int
		var published string
		
		rows.Scan(&id, &href, &mediatype, &name, &size, &published)

		re := regexp.MustCompile(`^\w+`)

		_type := re.FindString(mediatype)

		if _type == "image" {

			re = regexp.MustCompile(`.+/`)

			file := re.ReplaceAllString(mediatype, "")

			nHref := GetUniqueFilename(file)

			var nPreview NestedObjectBase

			re = regexp.MustCompile(`/\w+$`)
			actor := re.ReplaceAllString(id, "")
			
			nPreview.Type = "Preview"
			nPreview.Id = fmt.Sprintf("%s/%s", actor, CreateUniqueID(db, actor))		
			nPreview.Name = name
			nPreview.Href = Domain + "" + nHref
			nPreview.MediaType = mediatype
			nPreview.Size = int64(size)
			nPreview.Published = published
			nPreview.Updated = published		

			re = regexp.MustCompile(`/public/.+`)

			objFile := re.FindString(href)
			
			if(id != "") {
				cmd := exec.Command("convert", "." + objFile ,"-resize", "250x250>", "." + nHref)

				err := cmd.Run()

				CheckError(err, "error with resize attachment preview")

				if err == nil {
					fmt.Println(objFile + " -> " + nHref)				
					WritePreviewToDB(db, nPreview)
					UpdateObjectWithPreview(db, id, nPreview.Id)
				}			
			}
		}
	}
}

func UpdateObjectWithPreview(db *sql.DB, id string, preview string) {
	query := `update activitystream set preview=$1 where attachment=$2`

	_, err := db.Exec(query, preview, id)

	CheckError(err, "could not update activity stream with preview")

}
