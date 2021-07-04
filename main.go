package main

import (
	"fmt"
	"strings"
	"strconv"
	"net/http"
	"net/url"
	"database/sql"
	_ "github.com/lib/pq"
	"math/rand"
	"html/template"
	"time"
	"regexp"
	"os/exec"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"mime/multipart"
	"os"
	"bufio"
	"io"
	"github.com/gofrs/uuid"
)

var Port = ":" + GetConfigValue("instanceport")
var TP   = GetConfigValue("instancetp")
var Instance = GetConfigValue("instance")
var Domain = TP + "" + Instance

var authReq = []string{"captcha","email","passphrase"}

var supportedFiles = []string{"image/gif","image/jpeg","image/png", "image/webp", "image/apng","video/mp4","video/ogg","video/webm","audio/mpeg","audio/ogg","audio/wav", "audio/wave", "audio/x-wav"}

var SiteEmail = GetConfigValue("emailaddress")        //contact@fchan.xyz
var SiteEmailPassword = GetConfigValue("emailpass")
var SiteEmailServer = GetConfigValue("emailserver")   //mail.fchan.xyz
var SiteEmailPort = GetConfigValue("emailport")       //587

var TorProxy = GetConfigValue("torproxy") //127.0.0.1:9050

var PublicIndexing = strings.ToLower(GetConfigValue("publicindex"))

var Salt = GetConfigValue("instancesalt")

var activitystreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""

func main() {

	CreatedNeededDirectories()

	InitCache()

	db := ConnectDB();

	defer db.Close()

	RunDatabaseSchema(db)

	go MakeCaptchas(db, 100)

	*Key = CreateKey(32)

	FollowingBoards = GetActorFollowingDB(db, Domain)
	
	Boards = GetBoardCollection(db)

	// root actor is used to follow remote feeds that are not local
	//name, prefname, summary, auth requirements, restricted
	if GetConfigValue("instancename") != "" {
		CreateNewBoardDB(db, *CreateNewActor("", GetConfigValue("instancename"), GetConfigValue("instancesummary"), authReq, false))
		if PublicIndexing == "true" {
			AddInstanceToIndex(Domain)			
		}
	}
	
	// Allow access to public media folder
	fileServer := http.FileServer(http.Dir("./public"))
	http.Handle("/public/", http.StripPrefix("/public", neuter(fileServer)))

	javascriptFiles := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static", neuter(javascriptFiles)))					

	// main routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
		path := r.URL.Path

		// remove trailing slash
		if path != "/" {
			re := regexp.MustCompile(`/$`)
			path = re.ReplaceAllString(path, "")
		}

		var mainActor bool
		var mainInbox bool
		var mainOutbox bool
		var mainFollowing bool
		var mainFollowers bool		

		var actorMain bool
		var actorInbox bool
		var actorCatalog bool
		var actorOutbox bool
		var actorPost bool
		var actorFollowing bool
		var actorFollowers bool
		var actorReported bool		
		var actorVerification bool
		var actorMainPage bool

		var accept = r.Header.Get("Accept")
		
		var method = r.Method

		var actor = GetActorFromPath(db, path, "/")

		if actor.Name == "main" {
			mainActor = (path == "/")			
			mainInbox = (path == "/inbox")
			mainOutbox = (path == "/outbox")
			mainFollowing = (path == "/following")
			mainFollowers = (path == "/followers")			
		} else {
			actorMain = (path == "/" + actor.Name)
			actorInbox = (path == "/" + actor.Name + "/inbox")
			actorCatalog = (path == "/" + actor.Name + "/catalog")
			actorOutbox = (path == "/" + actor.Name + "/outbox")
			actorFollowing = (path == "/" + actor.Name + "/following")
			actorFollowers = (path == "/" + actor.Name + "/followers")
			actorReported = (path == "/" + actor.Name + "/reported")				
			actorVerification = (path == "/" + actor.Name + "/verification")

			escapedActorName := strings.Replace(actor.Name, "*", "\\*", -1)
			escapedActorName = strings.Replace(escapedActorName, "^", "\\^", -1)
			escapedActorName = strings.Replace(escapedActorName, "$", "\\$", -1)
			escapedActorName = strings.Replace(escapedActorName, "?", "\\?", -1)
			escapedActorName = strings.Replace(escapedActorName, "+", "\\+", -1)
			escapedActorName = strings.Replace(escapedActorName, ".", "\\.", -1)															

			re := regexp.MustCompile("/" + escapedActorName + "/[0-9]{1,2}$")

			actorMainPage = re.MatchString(path)
			
			re = regexp.MustCompile("/" + escapedActorName + "/\\w+")
			
			actorPost = 	re.MatchString(path)
		}

		if mainActor {
			if acceptActivity(accept) {
				GetActorInfo(w, db, Domain)
				return
			}

			IndexGet(w, r, db)
			
			return
		}
		
		if mainInbox {
			if method == "POST" {
				
			} else {
				w.WriteHeader(http.StatusForbidden)				
				w.Write([]byte("404 no path"))
			}
			return
		}
		
		if mainOutbox {
			if method == "GET" {
				GetActorOutbox(w, r, db)
			} else if method == "POST" {
				ParseOutboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)			
				w.Write([]byte("404 no path"))
			}
			return
		}

		if mainFollowing {
			GetActorFollowing(w, db, Domain)
			return
		}

		if mainFollowers {
			GetActorFollowers(w, db, Domain)
			return
		}

		if actorMain || actorMainPage {
			if acceptActivity(accept) {
				GetActorInfo(w, db, actor.Id)
				return 
			}

			postNum := strings.Replace(r.URL.EscapedPath(), "/" + actor.Name + "/", "", 1)

			page, _ := strconv.Atoi(postNum)			
			collection, valid := WantToServePage(db, actor.Name, page)

			if valid {
				OutboxGet(w, r, db, collection)
			}			

			return
		}

		if actorFollowing {
			GetActorFollowing(w, db, actor.Id)
			return
		}

		if actorFollowers {
			GetActorFollowers(w, db, actor.Id)
			return
		}		

		if actorInbox {
			if method == "POST" {
				ParseInboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("404 no path"))							
			}
			return
		}

		if actorCatalog {
			collection, valid := WantToServeCatalog(db, actor.Name)
			if valid {
				CatalogGet(w, r, db, collection)
			}
			return
		}

		if actorOutbox {
			if method == "GET" {
				GetActorOutbox(w, r, db)				
			} else if method == "POST" {
				ParseOutboxRequest(w, r, db)
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("404 no path"))											
			}
			return
		}

		if actorReported {
			GetActorReported(w, r, db, actor.Id)
			return
		}

		if actorVerification {
			r.ParseForm()

			code := r.FormValue("code")

			var verify Verify

			verify.Board = actor.Id
			verify.Identifier = "post"

			verify = GetVerificationCode(db, verify)

			auth := CreateTripCode(verify.Code)
			auth = CreateTripCode(auth)
		
			if CreateTripCode(auth) == code {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
			
			w.Write([]byte(""))																										
		}

		//catch all
		if actorPost {
			if acceptActivity(accept) {			
				GetActorPost(w, db, path)
				return 
			}

			PostGet(w, r, db)
			return			
		}		

		w.WriteHeader(http.StatusForbidden)			
		w.Write([]byte("404 no path"))
	})
	
	http.HandleFunc("/news/", func(w http.ResponseWriter, r *http.Request){
		timestamp := r.URL.Path[6:]

		if(len(timestamp) < 2) {
			AllNewsGet(w, r, db)
			return
		}

		if timestamp[len(timestamp)-1:] == "/" {
			timestamp = timestamp[:len(timestamp)-1]
		}
		
		ts, err := strconv.Atoi(timestamp)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)			
			w.Write([]byte("404 no path"))
		} else {
			NewsGet(w, r, db, ts)
		}
	})

	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request){

		r.ParseMultipartForm(10 << 20)

		file, header, _ := r.FormFile("file")

		if(file != nil && header.Size > (7 << 20)){
			w.Write([]byte("7MB max file size"))
			return
		}

		if(r.FormValue("inReplyTo") == "" && file == nil) {
				w.Write([]byte("Media is required for new posts"))
				return			
		}
		

		if(r.FormValue("inReplyTo") == "" || file == nil) {
			if(r.FormValue("comment") == "" && r.FormValue("subject") == ""){
				w.Write([]byte("Comment or Subject required"))
				return
			}
		}

		if(len(r.FormValue("comment")) > 2000) {
			w.Write([]byte("Comment limit 2000 characters"))
			return
		}

		if(len(r.FormValue("subject")) > 100 || len(r.FormValue("name")) > 100 || len(r.FormValue("options")) > 100) {
			w.Write([]byte("Name, Subject or Options limit 100 characters"))
			return
		}		

		if(r.FormValue("captcha") == "") {
			w.Write([]byte("Incorrect Captcha"))
			return
		}			
		
		b := bytes.Buffer{}
		we := multipart.NewWriter(&b)

		if(file != nil){
			var fw io.Writer
			
			fw, err := we.CreateFormFile("file", header.Filename)

			CheckError(err, "error with form file create")

			_, err = io.Copy(fw, file)
			
			CheckError(err, "error with form file copy")
		}

		reply := ParseCommentForReply(r.FormValue("comment"))

		for key, r0 := range r.Form {
			if(key == "captcha") {
				err := we.WriteField(key, r.FormValue("captchaCode") + ":" + r.FormValue("captcha"))
				CheckError(err, "error with writing captcha field")
			}else if(key == "name") {
				name, tripcode := CreateNameTripCode(r, db)
				err := we.WriteField(key, name)
				CheckError(err, "error with writing name field")
				err = we.WriteField("tripcode", tripcode)
				CheckError(err, "error with writing tripcode field")				
			}else{
				err := we.WriteField(key, r0[0])
				CheckError(err, "error with writing field")
			}
		}
		
		if(r.FormValue("inReplyTo") == "" && reply != ""){
			err := we.WriteField("inReplyTo", reply)
			CheckError(err, "error with writing inReplyTo field")			
		}
		
		we.Close()

		sendTo := r.FormValue("sendTo")		
		req, err := http.NewRequest("POST", sendTo, &b)

		CheckError(err, "error with post form req")

		req.Header.Set("Content-Type", we.FormDataContentType())

		resp, err := RouteProxy(req)

		CheckError(err, "error with post form resp")

		defer resp.Body.Close()

		if(resp.StatusCode == 200){
			
			body, _ := ioutil.ReadAll(resp.Body)
			
			var obj ObjectBase

			obj = ParseOptions(r, obj)
			for _, e := range obj.Option {
				if(e == "noko" || e == "nokosage"){
					http.Redirect(w, r, Domain + "/" + r.FormValue("boardName") + "/" + shortURL(r.FormValue("sendTo"), string(body)) , http.StatusMovedPermanently)
					return					
				}
			}

			if(r.FormValue("returnTo") == "catalog"){
				http.Redirect(w, r, Domain + "/" + r.FormValue("boardName") + "/catalog", http.StatusMovedPermanently)
			} else {
				http.Redirect(w, r, Domain + "/" + r.FormValue("boardName"), http.StatusMovedPermanently)
			}
			return
		}

		if(resp.StatusCode == 403){
			w.Write([]byte("Incorrect Captcha"))
			return
		}
		
		http.Redirect(w, r, Domain + "/" + r.FormValue("boardName"), http.StatusMovedPermanently)
	})

	http.HandleFunc("/" + *Key + "/", func(w http.ResponseWriter, r *http.Request) {

		id, _ := GetPasswordFromSession(r)
		
		actor := GetActorFromPath(db, r.URL.Path, "/" + *Key + "/")

		if actor.Id == "" {
			actor = GetActorFromDB(db, Domain)
		}

		if id == "" || (id != actor.Id && id != Domain) {
			t := template.Must(template.ParseFiles("./static/verify.html"))
			t.Execute(w, "")
			return
		}

		re := regexp.MustCompile("/" + *Key + "/" + actor.Name + "/follow")
		follow := re.MatchString(r.URL.Path)

		re = regexp.MustCompile("/" + *Key + "/" + actor.Name)
		manage := re.MatchString(r.URL.Path)

		re = regexp.MustCompile("/" + *Key )
		admin := re.MatchString(r.URL.Path)

		re = regexp.MustCompile("/" + *Key + "/follow" )
		adminFollow := re.MatchString(r.URL.Path)

		if follow || adminFollow {
			r.ParseForm()


			var followActivity Activity

			followActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			followActivity.Type = "Follow"
			

			var obj ObjectBase
			var nactor Actor
			if r.FormValue("actor") == Domain {
				nactor = GetActorFromDB(db, r.FormValue("actor"))
			} else {
				nactor = FingerActor(r.FormValue("actor"))			
			}
			
			followActivity.Actor = &nactor
			followActivity.Object = &obj

			followActivity.Object.Actor = r.FormValue("follow")
			followActivity.To = append(followActivity.To, r.FormValue("follow"))

			if followActivity.Actor.Id == Domain && !IsActorLocal(db, followActivity.Object.Actor) {
				w.Write([]byte("main board can only follow local boards. Create a new board and then follow outside boards from it."))
				return
			}

			MakeActivityRequestOutbox(db, followActivity)

			var redirect string
			if(actor.Name != "main") {
				redirect = "/" + actor.Name
			}

			http.Redirect(w, r, "/" + *Key + "/" + redirect, http.StatusSeeOther)							
			
		} else if manage && actor.Name != "" {
			t := template.Must(template.New("").Funcs(template.FuncMap{
				"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/manage.html"))

			follow := GetActorCollection(actor.Following)
			follower := GetActorCollection(actor.Followers)
			reported := GetActorCollectionReq(r, actor.Id + "/reported")

			var following []string
			var followers []string
			var reports   []Report

			for _, e := range follow.Items {
				following = append(following, e.Id)
			}

			for _, e := range follower.Items {
				followers = append(followers, e.Id)
			}

			for _, e := range reported.Items {
				var r Report
				r.Count = int(e.Size)
				r.ID    = e.Id
				reports = append(reports, r)
			}

			localReports := GetLocalReportDB(db, actor.Name)

			for _, e := range localReports {
				var r Report
				r.Count = e.Count
				r.ID    = e.ID
				reports = append(reports, r)
			}			

			var adminData AdminPage
			adminData.Following = following
			adminData.Followers = followers
			adminData.Reported  = reports
			adminData.Domain = Domain
			adminData.IsLocal = IsActorLocal(db, actor.Id)

			adminData.Title = "Manage /" + actor.Name + "/"
			adminData.Boards = Boards
			adminData.Board.Name = actor.Name
			adminData.Board.Actor = actor
			adminData.Key = *Key
			adminData.Board.TP = TP

			adminData.Board.Post.Actor = actor.Id
			
			t.ExecuteTemplate(w, "layout", adminData)
			
		} else if admin || actor.Id == Domain {
			t := template.Must(template.New("").Funcs(template.FuncMap{
				"sub": func (i, j int) int { return i - j }}).ParseFiles("./static/main.html", "./static/nadmin.html"))						
	
			actor := GetActor(Domain)
			follow := GetActorCollection(actor.Following).Items
			follower := GetActorCollection(actor.Followers).Items

			var following []string
			var followers []string

			for _, e := range follow {
				following = append(following, e.Id)
			}

			for _, e := range follower {
				followers = append(followers, e.Id)
			}

			var adminData AdminPage
			adminData.Following = following
			adminData.Followers = followers
			adminData.Actor = actor.Id
			adminData.Key = *Key
			adminData.Domain = Domain
			adminData.Board.ModCred,_ = GetPasswordFromSession(r)

			adminData.Boards = Boards
			
			adminData.Board.Post.Actor = actor.Id			

			t.ExecuteTemplate(w, "layout",  adminData)				
		}
	})

	http.HandleFunc("/" + *Key + "/addboard", func(w http.ResponseWriter, r *http.Request) {

		id, _ := GetPasswordFromSession(r)
		
		actor := GetActorFromDB(db, Domain)


		if id == "" || (id != actor.Id && id != Domain) {
			t := template.Must(template.ParseFiles("./static/verify.html"))
			t.Execute(w, "")
			return
		}
		
		var newActorActivity Activity
		var board Actor
		r.ParseForm()

		var restrict bool
		if r.FormValue("restricted") == "True" {
			restrict = true
		} else {
			restrict = false
		}
		
		board.Name = r.FormValue("name")
		board.PreferredUsername = r.FormValue("prefname")
		board.Summary = r.FormValue("summary")
		board.Restricted = restrict

		newActorActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		newActorActivity.Type = "New"

		var nobj ObjectBase
		newActorActivity.Actor = &actor
		newActorActivity.Object = &nobj

		newActorActivity.Object.Alias = board.Name
		newActorActivity.Object.Name = board.PreferredUsername
		newActorActivity.Object.Summary = board.Summary
		newActorActivity.Object.Sensitive = board.Restricted		

		MakeActivityRequestOutbox(db, newActorActivity)
		http.Redirect(w, r, "/" + *Key, http.StatusSeeOther)		
	})
	
	http.HandleFunc("/" + *Key + "/postnews", func(w http.ResponseWriter, r *http.Request) {

		id, _ := GetPasswordFromSession(r)
		
		actor := GetActorFromDB(db, Domain)


		if id == "" || (id != actor.Id && id != Domain) {
			t := template.Must(template.ParseFiles("./static/verify.html"))
			t.Execute(w, "")
			return
		}
		
		var newsitem NewsItem
		
		newsitem.Title = r.FormValue("title")
		newsitem.Content = template.HTML(r.FormValue("summary"))
		
		WriteNewsToDB(db, newsitem)
		
		http.Redirect(w, r, "/", http.StatusSeeOther)		
	})
	
	http.HandleFunc("/" + *Key + "/newsdelete/", func(w http.ResponseWriter, r *http.Request){

		id, _ := GetPasswordFromSession(r)
		
		actor := GetActorFromDB(db, Domain)


		if id == "" || (id != actor.Id && id != Domain) {
			t := template.Must(template.ParseFiles("./static/verify.html"))
			t.Execute(w, "")
			return
		}
		
		timestamp := r.URL.Path[13+len(*Key):]
		
		tsint, err := strconv.Atoi(timestamp)
		
		if(err != nil){
			w.WriteHeader(http.StatusForbidden)			
			w.Write([]byte("404 no path"))
			return
		} else {
			deleteNewsItemFromDB(db, tsint)
			http.Redirect(w, r, "/news/", http.StatusSeeOther)
		}
	})

	http.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request){
		if(r.Method == "POST") {
			r.ParseForm()
			identifier := r.FormValue("id")
			code := r.FormValue("code")

			var verify Verify
			verify.Identifier = identifier
			verify.Code = code

			j, _ := json.Marshal(&verify)
			
			req, err := http.NewRequest("POST", Domain + "/auth", bytes.NewBuffer(j))

			CheckError(err, "error making verify req")

			req.Header.Set("Content-Type", activitystreams)			

			resp, err := http.DefaultClient.Do(req)

			CheckError(err, "error getting verify resp")

			defer resp.Body.Close()

			rBody, _ := ioutil.ReadAll(resp.Body)

			body := string(rBody)

			if(resp.StatusCode != 200) {
				t := template.Must(template.ParseFiles("./static/verify.html"))
				t.Execute(w, "wrong password " + verify.Code)			
			} else {
				
				sessionToken, _ := uuid.NewV4()

				_, err := cache.Do("SETEX", sessionToken, "86400", body + "|" + verify.Code)
				if err != nil {
					t := template.Must(template.ParseFiles("./static/verify.html"))
					t.Execute(w, "")			
					return
				}

				http.SetCookie(w, &http.Cookie{
					Name:    "session_token",
					Value:   sessionToken.String(),
					Expires: time.Now().UTC().Add(60 * 60 * 48 * time.Second),
				})

				http.Redirect(w, r, "/", http.StatusSeeOther)				
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("404 no path"))			
		}
	})			

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request){
		id := r.URL.Query().Get("id")
		board := r.URL.Query().Get("board")
		
		_, auth := GetPasswordFromSession(r)

		if id == "" || auth == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		manage := r.URL.Query().Get("manage")
		col := GetCollectionFromID(id)

		if len(col.OrderedItems) < 1 {
			if !HasAuth(db, auth, GetActorByNameFromDB(db, board).Id) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				return
			}
			
			if !CheckIfObjectOP(db, id) {
				TombstoneObject(db, id)
			} else {
				TombstoneObjectAndReplies(db, id)
			}

			if(manage == "t"){
				http.Redirect(w, r, "/" + *Key + "/" + board , http.StatusSeeOther)
				return
			} else {
				http.Redirect(w, r, "/" + board, http.StatusSeeOther)
				return
			}
		}
		
		actor := col.OrderedItems[0].Actor

		if !HasAuth(db, auth, actor) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		var obj ObjectBase
		obj.Id = id
		obj.Actor = actor
		
		isOP := CheckIfObjectOP(db, obj.Id)		

		var OP string
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		}

		if !isOP {
			TombstoneObject(db, id)
		} else {
			TombstoneObjectAndReplies(db, id)
		}

		if IsIDLocal(db, id){
			DeleteObjectRequest(db, id)
		}

		if(manage == "t"){
			http.Redirect(w, r, "/" + *Key + "/" + board , http.StatusSeeOther)
			return
		} else if !isOP {
			if (!IsIDLocal(db, id)){
				http.Redirect(w, r, "/" + board + "/" + remoteShort(OP), http.StatusSeeOther)
				return
			} else {
				http.Redirect(w, r, OP, http.StatusSeeOther)
				return
			}
		} else {
			http.Redirect(w, r, "/" + board, http.StatusSeeOther)
			return
		}
		
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))		
	})

	http.HandleFunc("/deleteattach", func(w http.ResponseWriter, r *http.Request){
		
		id := r.URL.Query().Get("id")
		board := r.URL.Query().Get("board")		

		_, auth := GetPasswordFromSession(r)

		if id == "" || auth == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}
		
		manage := r.URL.Query().Get("manage")		
		col := GetCollectionFromID(id)

		if len(col.OrderedItems) < 1 {
			if !HasAuth(db, auth, GetActorByNameFromDB(db, board).Id) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				return
			}
			
			DeleteAttachmentFromFile(db, id)
			TombstoneAttachmentFromDB(db, id)
			
			DeletePreviewFromFile(db, id)
			TombstonePreviewFromDB(db, id)			

			if(manage == "t"){
				http.Redirect(w, r, "/" + *Key + "/" + board , http.StatusSeeOther)
				return
			} else {
				http.Redirect(w, r, "/" + board, http.StatusSeeOther)
				return
			}
		}
		
		actor := col.OrderedItems[0].Actor

		var OP string
		if (len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "") {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = id
		}
		
		if !HasAuth(db, auth, actor) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		DeleteAttachmentFromFile(db, id)
		TombstoneAttachmentFromDB(db, id)
		
		DeletePreviewFromFile(db, id)
		TombstonePreviewFromDB(db, id)
		
		if (manage == "t") {
			http.Redirect(w, r, "/" + *Key + "/" + board, http.StatusSeeOther)
			return
		} else if !IsIDLocal(db, OP) {
			http.Redirect(w, r, "/" + board + "/" + remoteShort(OP), http.StatusSeeOther)
			return
		} else {
			http.Redirect(w, r, OP, http.StatusSeeOther)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))				
	})

	http.HandleFunc("/marksensitive", func(w http.ResponseWriter, r *http.Request){
		
		id := r.URL.Query().Get("id")
		board := r.URL.Query().Get("board")		

		_, auth := GetPasswordFromSession(r)

		if id == "" || auth == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}
		
		col := GetCollectionFromID(id)

		if len(col.OrderedItems) < 1 {
			if !HasAuth(db, auth, GetActorByNameFromDB(db, board).Id) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				return
			}

			MarkObjectSensitive(db, id, true)
			
			http.Redirect(w, r, "/" + board, http.StatusSeeOther)
			return
		}
		
		actor := col.OrderedItems[0].Actor

		var OP string
		if (len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "") {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = id
		}
		
		if !HasAuth(db, auth, actor) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		MarkObjectSensitive(db, id, true)

		if !IsIDLocal(db, OP) {
			http.Redirect(w, r, "/" + board + "/" + remoteShort(OP), http.StatusSeeOther)
			return
		} else {
			http.Redirect(w, r, OP, http.StatusSeeOther)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))				
	})	

	http.HandleFunc("/remove", func(w http.ResponseWriter, r *http.Request){
		id := r.URL.Query().Get("id")
		manage := r.URL.Query().Get("manage")
		board := r.URL.Query().Get("board")
		col := GetCollectionFromID(id)		
		actor := col.OrderedItems[0].Actor
		_, auth := GetPasswordFromSession(r)

		if id == "" || auth == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		if !HasAuth(db, auth, actor) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		var obj ObjectBase
		obj.Id = id
		obj.Actor = actor
		
		isOP := CheckIfObjectOP(db, obj.Id)		

		var OP string
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		}

		if !isOP {
			SetObject(db, id, "Removed")
		} else {
			SetObjectAndReplies(db, id, "Removed")
		}

		if(manage == "t"){
			http.Redirect(w, r, "/" + *Key + "/" + board , http.StatusSeeOther)
			return
		} else if !isOP {
			if (!IsIDLocal(db, id)){
				http.Redirect(w, r, "/" + board + "/" + remoteShort(OP), http.StatusSeeOther)
				return
			} else {
				http.Redirect(w, r, OP, http.StatusSeeOther)
				return
			}
		} else {
			http.Redirect(w, r, "/" + board, http.StatusSeeOther)
			return
		}
		
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))		
	})

	http.HandleFunc("/removeattach", func(w http.ResponseWriter, r *http.Request){
		
		id := r.URL.Query().Get("id")
		manage := r.URL.Query().Get("manage")		
		board := r.URL.Query().Get("board")		
		col := GetCollectionFromID(id)		
		actor := col.OrderedItems[0].Actor

		var OP string
		if (len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "") {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = id
		}
		
		_, auth := GetPasswordFromSession(r)

		if id == "" || auth == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		if !HasAuth(db, auth, actor) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		SetAttachmentFromDB(db, id, "Removed")
		SetPreviewFromDB(db, id, "Removed")
		
		if (manage == "t") {
			http.Redirect(w, r, "/" + *Key + "/" + board, http.StatusSeeOther)
			return
		} else if !IsIDLocal(db, OP) {
			http.Redirect(w, r, "/" + board + "/" + remoteShort(OP), http.StatusSeeOther)
			return
		} else {
			http.Redirect(w, r, OP, http.StatusSeeOther)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))				
	})	

	http.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request){

		r.ParseForm()

		id := r.FormValue("id")
		board := r.FormValue("board")
		reason := r.FormValue("comment")
		close := r.FormValue("close")

		actor := GetActorFromPath(db, id, "/")
		_, auth := GetPasswordFromSession(r)

		var captcha = r.FormValue("captchaCode") + ":" + r.FormValue("captcha")

		if len(reason) > 100 {
			w.Write([]byte("Report comment limit 100 characters"))
			return
		}

		if(close != "1" && !CheckCaptcha(db, captcha)) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("captcha required"))					
			return							
		}
		
		if close == "1" {
			if !HasAuth(db, auth, actor.Id) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				return
			}

			if !IsIDLocal(db, id) {
				CloseLocalReportDB(db, id, board)				
				http.Redirect(w, r, "/" + *Key + "/" + board, http.StatusSeeOther)
				return 
			}

			reported := DeleteReportActivity(db, id)
			if reported {
				http.Redirect(w, r, "/" + *Key + "/" + board, http.StatusSeeOther)							
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(""))
			return
		}

		if !IsIDLocal(db, id) {
			CreateLocalReportDB(db, id, board, reason)
			http.Redirect(w, r, "/" + board + "/" + remoteShort(id), http.StatusSeeOther)
			return			
		}
		
		reported := ReportActivity(db, id, reason)
		if reported {
			http.Redirect(w, r, id, http.StatusSeeOther)			
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(""))					
	})

	http.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request){
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

	http.HandleFunc("/.well-known/webfinger", func(w http.ResponseWriter, r *http.Request) {
		acct := r.URL.Query()["resource"]

		if(len(acct) < 1) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("resource needs a value"))		
			return
		}

		acct[0] = strings.Replace(acct[0], "acct:", "", -1)

		actorDomain := strings.Split(acct[0], "@")

		if(len(actorDomain) < 2) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("accpets only subject form of acct:board@instance"))		
			return
		}

		if !IsActorLocal(db, TP + "" + actorDomain[1] + "/" + actorDomain[0]) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("actor not local"))		
			return
		}

		var finger Webfinger
		var link WebfingerLink
		
		finger.Subject = "acct:" + actorDomain[0] + "@" + actorDomain[1]
		link.Rel = "self"
		link.Type = "application/activity+json"
		link.Href = TP + "" + actorDomain[1] + "/" + actorDomain[0]

		finger.Links = append(finger.Links, link)

		enc, _ := json.Marshal(finger)

		w.Header().Set("Content-Type", activitystreams)
		w.Write(enc)
		
	})

	http.HandleFunc("/addtoindex", func(w http.ResponseWriter, r *http.Request) {
		actor := r.URL.Query().Get("id")

		if Domain != "https://fchan.xyz" {
			return
		}
		


		go AddInstanceToIndexDB(db, actor)
	})

	fmt.Println("Server for " + Domain + " running on port " + Port)

	fmt.Println("Mod key: " + *Key)
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

	code := strings.Split(out.String(), " ")

	return code[0]
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
		actor = "main"
	}

	var nActor Actor

	nActor = 	GetActorByNameFromDB(db, actor)

	if nActor.Id == "" {
		nActor = 	GetActorByName(db, actor)
	}

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
	rand.Seed(time.Now().UTC().UnixNano())
	domain := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
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

func GetActorInfo(w http.ResponseWriter, db *sql.DB, id string) {
	actor := GetActorFromDB(db, id)
	enc, _ := json.MarshalIndent(actor, "", "\t")
	w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	w.Write(enc)
}

func GetActorPost(w http.ResponseWriter, db *sql.DB, path string) {
	collection := GetCollectionFromPath(db, Domain + "" + path)
	if len(collection.OrderedItems) > 0 {
		enc, _ := json.MarshalIndent(collection, "", "\t")							
		w.Header().Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		w.Write(enc)
	}
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	nObj.Type = objType
	nObj.Published = time.Now().UTC().Format(time.RFC3339)
	nObj.Updated = time.Now().UTC().Format(time.RFC3339)

	return nObj
}

func AddFollowersToActivity(db *sql.DB, activity Activity) Activity{

	activity.To = append(activity.To, activity.Actor.Id)
	
	for _, e := range activity.To {
		aFollowers := GetActorCollection(e + "/followers")
		for _, k := range aFollowers.Items {
			activity.To = append(activity.To, k.Id)
		}
	}

	var nActivity Activity
	
	for _, e := range activity.To {
		var alreadyTo = false
		for _, k := range nActivity.To {
			if e == k || e == activity.Actor.Id {				
				alreadyTo = true
			}
		}

		if !alreadyTo {
			nActivity.To = append(nActivity.To, e)
		}
	}	

	activity.To = nActivity.To	
	
	return activity
}

func CreateActivity(activityType string, obj ObjectBase) Activity {
	var newActivity Activity
	actor := FingerActor(obj.Actor)
	
	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = &actor
	newActivity.Object = &obj

	for _, e := range obj.To {
		if obj.Actor != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor != e {		
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

	cmd := exec.Command("convert", "." + objFile ,"-resize", "250x250>", "-strip","." + href)

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
	image.Published = time.Now().UTC().Format(time.RFC3339)

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
			reply.Published = time.Now().UTC().Format(time.RFC3339)
			validLinks = append(validLinks, *reply)
		}
	}

	return validLinks
}

func CheckValidActivity(id string) (Collection, bool) {

	re := regexp.MustCompile(`.+\.onion(.+)?`)
	if re.MatchString(id) {
		id = strings.Replace(id, "https", "http", 1)
	}

	req, err := http.NewRequest("GET", id, nil)
	
	if err != nil {
		fmt.Println("error with request")
		panic(err)
	}

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

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

	if id == "" {
		return respActor
	}

	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "error with getting actor req")

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

	if err != nil {
		fmt.Println("error with getting actor resp " + id)
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

	if collection == "" {
		return nCollection
	}

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req " + collection)

	req.Header.Set("Accept", activitystreams)

	resp, err := RouteProxy(req)

	if err != nil {
		fmt.Println("error with getting actor collection resp " + collection)		
		return nCollection
	}


	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 { 
			err = json.Unmarshal(body, &nCollection)
			
			CheckError(err, "error getting actor collection from body " + collection)
		}
	}
	
	return nCollection
}

func IsValidActor(id string) (Actor, bool) {

	actor := FingerActor(id)

	if actor.Id != "" {
		return actor, true;
	}

	return actor, false;	
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
	activity := GetActivityFromDB(db, id)
	return len(activity.OrderedItems) > 0 
}

func IsActorLocal(db *sql.DB, id string) bool {
	actor := GetActorFromDB(db, id)

	if actor.Id != "" {
		return true
	}

	return false
}

func IsObjectLocal(db *sql.DB, id string) bool {

	query := `select id from activitystream where id=$1`

	rows, _ := db.Query(query, id)

	var nID string
	defer rows.Close()
	rows.Next()
	rows.Scan(&nID)

	if nID == "" {
		return false
	}

	return true
}

func IsObjectCached(db *sql.DB, id string) bool {

	query := `select id from cacheactivitystream where id=$1`
	rows, _ := db.Query(query, id)

	var nID string
	defer rows.Close()
	rows.Next()
	rows.Scan(&nID)	

	if nID == "" {
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

	query := `delete from reported where id=$1`

	_, err := db.Exec(query, id)

	if err != nil {
		CheckError(err, "error closing reported activity")
		return false
	}
	
	return true
}

func ReportActivity(db *sql.DB, id string, reason string) bool {

	if !IsIDLocal(db, id) {
		return false
	}

	actor := GetActivityFromDB(db, id)

	query := `select count from reported where id=$1`
	
	rows, err := db.Query(query, id)

	CheckError(err, "could not select count from reported")

	defer rows.Close()
	var count int
	for rows.Next() {
		rows.Scan(&count)
	}

	if count < 1 {
		query = `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`

		_, err := db.Exec(query, id, 1, actor.Actor.Id, reason)

		if err != nil {
			CheckError(err, "error inserting new reported activity")
			return false
		}
		
	} else {
		count = count + 1
		query = `update reported set count=$1 where id=$2`

		_, err := db.Exec(query, count, id)
		
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
	w.Header().Set("Content-Type", activitystreams)
	w.Write(enc)
}

func MakeActivityRequestOutbox(db *sql.DB, activity Activity) {
	j, _ := json.Marshal(activity)

	if activity.Actor.Outbox == "" {
		return
	}

	req, err := http.NewRequest("POST", activity.Actor.Outbox, bytes.NewBuffer(j))

	CheckError(err, "error with sending activity req to outbox")		

	re := regexp.MustCompile("https?://(www.)?")
	
	var instance string
	if activity.Actor.Id == Domain {
		instance = re.ReplaceAllString(Domain, "")
	} else {
		_, instance = GetActorInstance(activity.Actor.Id)
	}

	date := time.Now().UTC().Format(time.RFC1123)
	path := strings.Replace(activity.Actor.Outbox, instance, "", 1)


	path = re.ReplaceAllString(path, "")

	sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
	encSig, err := ActivitySign(db, *activity.Actor, sig)
	CheckError(err, "unable to sign activity response")
	signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)	
	
	req.Header.Set("Content-Type", activitystreams)			
	req.Header.Set("Date", date)
	req.Header.Set("Signature", signature)
	req.Host = instance

	_, err = RouteProxy(req)

	CheckError(err, "error with sending activity resp to")	
}

func MakeActivityRequest(db *sql.DB, activity Activity) {

	j, _ := json.MarshalIndent(activity, "", "\t")

	for _, e := range activity.To {
		if e != activity.Actor.Id {
			
			actor := FingerActor(e)

			if actor.Id != "" {
				_, instance := GetActorInstance(actor.Id)

				if actor.Inbox != "" {

					req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))

					CheckError(err, "error with sending activity req to")

					date := time.Now().UTC().Format(time.RFC1123)
					path := strings.Replace(actor.Inbox, instance, "", 1)
					
					re := regexp.MustCompile("https?://(www.)?")
					path = re.ReplaceAllString(path, "")

					sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
					encSig, err := ActivitySign(db, *activity.Actor, sig)
					CheckError(err, "unable to sign activity response")
					signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

					req.Header.Set("Content-Type", activitystreams)			
					req.Header.Set("Date", date)
					req.Header.Set("Signature", signature)
					req.Host = instance

					_, err = RouteProxy(req)

					CheckError(err, "error with sending activity resp to")
				}				
			}
		}
	}	
}

func GetCollectionFromID(id string) Collection {
	var nColl Collection
	
	req, err := http.NewRequest("GET", id, nil)

	CheckError(err, "could not get collection from id req")

	req.Header.Set("Accept", activitystreams)				
	
	resp, err := RouteProxy(req)

	if err != nil {
		return nColl
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if len(body) > 0 {
			err = json.Unmarshal(body, &nColl)

			CheckError(err, "error getting collection resp from json body")
		}
	}

	return nColl
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
	var nActor Actor
	nObj.Id = id
	nObj.Actor = nActor.Id

	activity := CreateActivity("Delete", nObj)

	obj := GetObjectFromPath(db, id)

	actor := FingerActor(obj.Actor)
	activity.Actor = &actor

	followers := GetActorFollowDB(db, obj.Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(db, obj.Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}

	MakeActivityRequest(db, activity)
}

func DeleteObjectAndRepliesRequest(db *sql.DB, id string) {
	var nObj ObjectBase
	var nActor Actor	
	nObj.Id = id
	nObj.Actor = nActor.Id
	
	activity := CreateActivity("Delete", nObj)
	
	obj := GetObjectByIDFromDB(db, id)

	activity.Actor.Id = obj.OrderedItems[0].Actor

	activity.Object = &obj.OrderedItems[0]
	
	followers := GetActorFollowDB(db, obj.OrderedItems[0].Actor)
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following := GetActorFollowingDB(db, obj.OrderedItems[0].Actor)
	for _, e := range following {
		activity.To = append(activity.To, e.Id)
	}	

	MakeActivityRequest(db, activity)
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
				cmd := exec.Command("convert", "." + objFile ,"-resize", "250x250>", "-strip",  "." + nHref)

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

func ParseCommentForReply(comment string) string {
	
	re := regexp.MustCompile("(>>)(https://|http://)?(www\\.)?.+\\/\\w+")	
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i:= 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		links = append(links, str)
	}

	if(len(links) > 0){
		_, isValid := CheckValidActivity(links[0])

		if(isValid) {
			return links[0]
		}
	}
	
	return ""
}

func GetActorByName(db *sql.DB, name string) Actor {
	var actor Actor
		for _, e := range Boards {
			if e.Actor.Name == name {
				actor = e.Actor
			}
		}

	return actor
}

func GetActorCollectionReq(r *http.Request, collection string) Collection {
	var nCollection Collection

	req, err := http.NewRequest("GET", collection, nil)

	CheckError(err, "error with getting actor collection req " + collection)

	_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", activitystreams)

	req.Header.Set("Authorization", "Basic " + pass)		

	resp, err := RouteProxy(req)

	CheckError(err, "error with getting actor collection resp " + collection)

	if resp.StatusCode == 200 {

		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		err = json.Unmarshal(body, &nCollection)

		CheckError(err, "error getting actor collection from body " + collection)
	}
	
	return nCollection
}


func shortURL(actorName string, url string) string {

	re := regexp.MustCompile(`.+\/`)

	actor := re.FindString(actorName)

	urlParts := strings.Split(url, "|")

	op := urlParts[0]

	var reply string

	if len(urlParts) > 1 {
		reply = urlParts[1]
	}

	re = regexp.MustCompile(`\w+$`)
	temp := re.ReplaceAllString(op, "")

	if(temp == actor){
		id := localShort(op)

    re := regexp.MustCompile(`.+\/`)
    replyCheck := re.FindString(reply)

		if(reply != "" && replyCheck == actor){
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}

		return id;            
	}else{
		id := remoteShort(op)

    re := regexp.MustCompile(`.+\/`)
    replyCheck := re.FindString(reply)

		if(reply != "" && replyCheck == actor){
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}		

		return id;                        
	}
}

func localShort(url string) string {
		re := regexp.MustCompile(`\w+$`)
		return re.FindString(StripTransferProtocol(url));
}

func remoteShort(url string) string {
	re := regexp.MustCompile(`\w+$`)

	id := re.FindString(StripTransferProtocol(url));

	re = regexp.MustCompile(`.+/.+/`)

	actorurl := re.FindString(StripTransferProtocol(url))

	re = regexp.MustCompile(`/.+/`)

	actorname := re.FindString(actorurl)

	actorname = strings.Replace(actorname, "/", "", -1)

	return "f" + actorname + "-" + id
}

func RouteProxy(req *http.Request) (*http.Response, error) {

	var proxyType = GetPathProxyType(req.URL.Host)

	if proxyType == "tor" {
		proxyUrl, err := url.Parse("socks5://" + TorProxy)

		CheckError(err, "error parsing tor proxy url")

		proxyTransport := &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		client := &http.Client{ Transport: proxyTransport, Timeout: time.Second * 75 }
		return client.Do(req)
	}

	return http.DefaultClient.Do(req)
}

func GetPathProxyType(path string) string {
	if TorProxy != "" {
		re := regexp.MustCompile(`(http://|http://)?(www.)?\w+\.onion`)
		onion := re.MatchString(path)
		if onion {
			return "tor"
		}
	}

	return "clearnet"
}

func RunDatabaseSchema(db *sql.DB) {
	query, err := ioutil.ReadFile("databaseschema.psql")
	CheckError(err, "could not read databaseschema.psql file")
	if _, err := db.Exec(string(query)); err != nil {
		CheckError(err, "could not exec databaseschema.psql")		
	}
}

func CreatedNeededDirectories() {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
    os.Mkdir("./public", 0755)
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
    os.MkdirAll("./pem/board", 0700)
	}	
}

//looks for actor with pattern of board@instance
func FingerActor(path string) Actor{

	actor, instance := GetActorInstance(path)

	r := FingerRequest(actor, instance)

	var nActor Actor

	if r != nil && r.StatusCode == 200 {
		defer r.Body.Close()
		
		body, _ := ioutil.ReadAll(r.Body)

		err := json.Unmarshal(body, &nActor)

		CheckError(err, "error getting fingerrequet resp from json body")
	}	

	return nActor
}

func FingerRequest(actor string, instance string) (*http.Response){
	acct := "acct:" + actor + "@" + instance
	req, err := http.NewRequest("GET", "http://" + instance + "/.well-known/webfinger?resource=" + acct, nil)

	CheckError(err, "could not get finger request from id req")

	resp, err := RouteProxy(req)

	var finger Webfinger

	if err != nil {
		return resp
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()
		
		body, _ := ioutil.ReadAll(resp.Body)

		err := json.Unmarshal(body, &finger)

		CheckError(err, "error getting fingerrequet resp from json body")
	}

	if(len(finger.Links) > 0) {
		for _, e := range finger.Links {
			if(e.Type == "application/activity+json"){
				req, err := http.NewRequest("GET", e.Href, nil)
				
				CheckError(err, "could not get finger request from id req")

				req.Header.Set("Accept", activitystreams)				
				
				resp, err := RouteProxy(req)
				return resp
			}
		}
	}

	return resp
}

func GetActorInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if(atFormat) {
		match := re.FindStringSubmatch(path)
		if(len(match) > 2) {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?:\\)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if(httpFormat) {
		match := re.FindStringSubmatch(path)
		if(len(match) > 3) {
			if match[4] == "users" {
				return match[6], match[3]
			}
			
			return match[4], match[3]
		}
	}

	return "", ""
}

func AddInstanceToIndex(actor string) {
	// if local testing enviroment do not add to index
	re := regexp.MustCompile(`(.+)?(localhost|\d+\.\d+\.\d+\.\d+)(.+)?`)
	if re.MatchString(actor) {
		return
	}
	
	followers := GetCollectionFromID("https://fchan.xyz/followers")

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == actor {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		req, err := http.NewRequest("GET", "https://fchan.xyz/addtoindex?id=" + actor, nil)

		CheckError(err, "error with add instance to actor index req")

		_, err = http.DefaultClient.Do(req)

		CheckError(err, "error with add instance to actor index resp")		
	}
}

func AddInstanceToIndexDB(db *sql.DB, actor string) {

	time.Sleep(15 * time.Second)
	
	followers := GetCollectionFromID("https://fchan.xyz/followers")

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == actor {
			alreadyIndex = true
		}
	}

	checkActor := GetActor(actor)

	if checkActor.Id == actor {
		if !alreadyIndex {			
			query := `insert into follower (id, follower) values ($1, $2)`

			_, err := db.Exec(query, "https://fchan.xyz", actor)

			CheckError(err, "Error with add to index query")
		}
	}
}

func GetCollectionFromReq(path string) Collection {
	req, err := http.NewRequest("GET", path, nil)

	CheckError(err, "error with getting collection from req")

	req.Header.Set("Accept", activitystreams)

	resp, err := http.DefaultClient.Do(req)

	CheckError(err, "error getting resp from collection req")

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respCollection Collection

	err = json.Unmarshal(body, &respCollection)

	if err != nil {
		panic(err)
	}	

	return respCollection
}

