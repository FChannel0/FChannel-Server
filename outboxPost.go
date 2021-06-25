package main

import "fmt"
import "net/http"
import "database/sql"
import _ "github.com/lib/pq"
import "encoding/json"
import "reflect"
import "io/ioutil"
import "os"
import "regexp"
import "strings"
import "os/exec"

func ParseOutboxRequest(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	
	var activity Activity

	actor := GetActorFromPath(db, r.URL.Path, "/")
	contentType := GetContentType(r.Header.Get("content-type"))

	defer r.Body.Close()
	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		r.ParseMultipartForm(5 << 20)		
		if(BoardHasAuthType(db, actor.Name, "captcha") && CheckCaptcha(db, r.FormValue("captcha"))) {		
			f, header, _ := r.FormFile("file")
			if(header != nil) {
				if(header.Size > (7 << 20)){
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					w.Write([]byte("7MB max file size"))
					return
				}
				
				contentType, _ := GetFileContentType(f)

				if(!SupportedMIMEType(contentType)) {
					w.WriteHeader(http.StatusNotAcceptable)
					w.Write([]byte("file type not supported"))
					return
				}
			}

			var nObj = CreateObject("Note")
			nObj = ObjectFromForm(r, db, nObj)
			
			nObj.Actor = Domain + "/" + actor.Name

			nObj = WriteObjectToDB(db, nObj)
			activity := CreateActivity("Create", nObj)
			activity = AddFollowersToActivity(db, activity)
			MakeActivityRequest(db, activity)

			var id string
			op := len(nObj.InReplyTo) - 1
			if op >= 0 {		
				if nObj.InReplyTo[op].Id == "" {
					id = nObj.Id  
				} else {
					id = nObj.InReplyTo[0].Id + "|" + nObj.Id
				}
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(id))
			return
		}

		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("captcha could not auth"))
	} else {
		activity = GetActivityFromJson(r, db)
		if IsActivityLocal(db, activity) {
			if !VerifyHeaderSignature(r, *activity.Actor) {
				w.WriteHeader(http.StatusBadRequest)										
				w.Write([]byte(""))					
				return
			}
			
			switch activity.Type {
			case "Create":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				break
				
			case "Follow":
				var validActor bool
				var validLocalActor bool

				validActor = (activity.Object.Actor != "")
				validLocalActor = (activity.Actor.Id == actor.Id)

				var rActivity Activity
				if validActor && validLocalActor {
					rActivity = AcceptFollow(activity)
					SetActorFollowingDB(db, rActivity)
					MakeActivityRequest(db, activity)
				}

				FollowingBoards = GetActorFollowingDB(db, Domain)
				Boards = GetBoardCollection(db)
				break
				
			case "Delete":
				fmt.Println("This is a delete")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))										
				break
				
			case "Note":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))
				break

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive

				actor := CreateNewBoardDB(db, *CreateNewActor(name, prefname, summary, authReq, restricted))
				
				if actor.Id != "" {
					var board []ObjectBase
					var item ObjectBase			
					var removed bool = false

					item.Id = actor.Id
					for _, e := range FollowingBoards {
						if e.Id != item.Id {
							board = append(board, e)
						} else {
							removed = true
						}
					}

					if !removed {
						board = append(board, item)
					}

					FollowingBoards = board
					Boards = GetBoardCollection(db)
					return
				}
				
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(""))
				break
				
			default:
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("could not process activity"))			
			}
		} else {
			fmt.Println("is NOT activity")		
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("could not process activity"))			
		}
	}
}

func ObjectFromJson(r *http.Request, obj ObjectBase) ObjectBase {
	body, _ := ioutil.ReadAll(r.Body)

	var respActivity ActivityRaw

	err := json.Unmarshal(body, &respActivity)

	CheckError(err, "error with object from json")

	if HasContextFromJson(respActivity.AtContextRaw.Context) {
		var jObj ObjectBase
		jObj = GetObjectFromJson(respActivity.ObjectRaw)
		jObj.To = GetToFromJson(respActivity.ToRaw)
		jObj.Cc = GetToFromJson(respActivity.CcRaw)
	}
	
	return obj
}

func GetObjectFromJson(obj []byte) ObjectBase {
	var generic interface{}

	err := json.Unmarshal(obj, &generic)

	CheckError(err, "error with getting obj from json")

	t := reflect.TypeOf(generic)

	var nObj ObjectBase
	if t != nil {
		switch t.String() {
		case "[]interface {}":
			var lObj ObjectBase		
			var arrContext ObjectArray
			err = json.Unmarshal(obj, &arrContext.Object)
			CheckError(err, "error with []interface{} oject from json")
			if len(arrContext.Object) > 0 {
				lObj = arrContext.Object[0]
			}
			nObj = lObj
			break

		case "map[string]interface {}":
			var arrContext Object
			err = json.Unmarshal(obj, &arrContext.Object)
			CheckError(err, "error with object from json")
			nObj = *arrContext.Object
			break
			
		case "string":
			var lObj ObjectBase
			var arrContext ObjectString
			err = json.Unmarshal(obj, &arrContext.Object)
			CheckError(err, "error with string object from json")
			lObj.Id = arrContext.Object
			nObj = lObj
			break
		}
	}

	return nObj
}

func GetActorFromJson(actor []byte) Actor{
	var generic interface{}
	var nActor Actor		
	err := json.Unmarshal(actor, &generic)

	if err != nil {
		return nActor
	}

	t := reflect.TypeOf(generic)
	if t != nil {
		switch t.String() {
		case "map[string]interface {}":
			err = json.Unmarshal(actor, &nActor)
			CheckError(err, "error with To []interface{}")
			
		case "string":
			var str string
			err = json.Unmarshal(actor, &str)
			CheckError(err, "error with To string")
			nActor.Id = str
		}
		
		return nActor		
	}
	
	return nActor
}

func GetToFromJson(to []byte) []string {
	var generic interface{}

	err := json.Unmarshal(to, &generic)

	if err != nil {
		return nil
	}

	t := reflect.TypeOf(generic)

	if t != nil {
		var nStr []string		
		switch t.String() {
		case "[]interface {}":
			err = json.Unmarshal(to, &nStr)
			CheckError(err, "error with To []interface{}")
			return nStr
			
		case "string":
			var str string
			err = json.Unmarshal(to, &str)
			CheckError(err, "error with To string")
			nStr = append(nStr, str)
			return nStr			
		}
	}
	
	return nil
}

func HasContextFromJson(context []byte) bool {
	var generic interface{}

	err := json.Unmarshal(context, &generic)

	CheckError(err, "error with getting context")

	t := reflect.TypeOf(generic)

	hasContext := false

	switch t.String() {
	case "[]interface {}":
		var arrContext AtContextArray
		err = json.Unmarshal(context, &arrContext.Context)
		CheckError(err, "error with []interface{}")
		if len(arrContext.Context) > 0 {
			if arrContext.Context[0] == "https://www.w3.org/ns/activitystreams" {
				hasContext = true
			}
		}
	case "string":
		var arrContext AtContextString
		err = json.Unmarshal(context, &arrContext.Context)
		CheckError(err, "error with string")
		if arrContext.Context == "https://www.w3.org/ns/activitystreams" {
			hasContext = true
		}
	}
	
	return hasContext
}

func ObjectFromForm(r *http.Request, db *sql.DB, obj ObjectBase) ObjectBase {
	
	file, header, _ := r.FormFile("file")
	
	if file != nil {
		defer file.Close()

		var tempFile = new(os.File)
		obj.Attachment, tempFile = CreateAttachmentObject(file, header)

		defer tempFile.Close();

		fileBytes, _ := ioutil.ReadAll(file)

		tempFile.Write(fileBytes)

		re := regexp.MustCompile(`image/(jpe?g|png|webp)`)
		if re.MatchString(obj.Attachment[0].MediaType) {
			fileLoc := strings.ReplaceAll(obj.Attachment[0].Href, Domain, "")

			cmd := exec.Command("exiv2", "rm", "." + fileLoc)

			err := cmd.Run()

			CheckError(err, "error with removing exif data from image")

		}

		obj.Preview = CreatePreviewObject(obj.Attachment[0])
	}

	obj.AttributedTo = EscapeString(r.FormValue("name"))
	obj.TripCode = EscapeString(r.FormValue("tripcode"))
	obj.Name = EscapeString(r.FormValue("subject"))
	obj.Content = EscapeString(r.FormValue("comment"))
	obj.Sensitive = (r.FormValue("sensitive") != "")

	obj = ParseOptions(r, obj)

	var originalPost ObjectBase
	originalPost.Id = EscapeString(r.FormValue("inReplyTo"))

	obj.InReplyTo = append(obj.InReplyTo, originalPost)

	var activity Activity

	if !IsInStringArray(activity.To, originalPost.Id) {
		activity.To = append(activity.To, originalPost.Id)
	}	

	if originalPost.Id != "" {
		if !IsActivityLocal(db, activity) {
			id := FingerActor(originalPost.Id).Id
			actor := GetActor(id)
			if !IsInStringArray(obj.To, actor.Id) {
				obj.To = append(obj.To, actor.Id)
			}
		}
	}

	replyingTo := ParseCommentForReplies(r.FormValue("comment"))

	for _, e := range replyingTo {

		has := false
		
		for _, f := range obj.InReplyTo {
			if e.Id == f.Id {
				has = true
				break
			}
		}
		
		if !has {
			obj.InReplyTo = append(obj.InReplyTo, e)

			var activity Activity
			
			activity.To = append(activity.To, e.Id)
			
			if !IsActivityLocal(db, activity) {
				id := FingerActor(e.Id).Id
				actor := GetActor(id)
				if !IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}				
			}
		}
	}

	return obj
}

func ParseOptions(r *http.Request, obj ObjectBase) ObjectBase {
	options := EscapeString(r.FormValue("options"))
	if options != "" {
		option := strings.Split(options, ";")
		email := regexp.MustCompile(".+@.+\\..+")		
		wallet := regexp.MustCompile("wallet:.+")
		delete := regexp.MustCompile("delete:.+")
		for _, e := range option {
			if e == "noko" {
				obj.Option = append(obj.Option, "noko")				 
			} else if e == "sage" {
				obj.Option = append(obj.Option, "sage")				 				
			} else if e == "nokosage" {
				obj.Option = append(obj.Option, "nokosage")				 								
			} else if email.MatchString(e) {
				obj.Option = append(obj.Option, "email:" + e)				 												
			} else if wallet.MatchString(e) {
				obj.Option = append(obj.Option, "wallet")				 																
				var wallet CryptoCur
				value := strings.Split(e, ":")
				wallet.Type = value[0]
				wallet.Address = value[1]
				obj.Wallet = append(obj.Wallet, wallet)
			} else if delete.MatchString(e) {
				obj.Option = append(obj.Option, e)
			}
		}
	}

	return obj
}

func GetActivityFromJson(r *http.Request, db *sql.DB) Activity {
	body, _ := ioutil.ReadAll(r.Body)

	var respActivity ActivityRaw

	var nActivity Activity

	var nType string

	err := json.Unmarshal(body, &respActivity)

	CheckError(err, "error with activity from json")

	if HasContextFromJson(respActivity.AtContextRaw.Context) {
		var jObj ObjectBase

		if respActivity.Type == "Note" {
			jObj = GetObjectFromJson(body)
			nType = "Create"
		} else {
			jObj = GetObjectFromJson(respActivity.ObjectRaw)
			nType = respActivity.Type
		}

		actor := GetActorFromJson(respActivity.ActorRaw)
		to := GetToFromJson(respActivity.ToRaw)
		cc := GetToFromJson(respActivity.CcRaw)

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
	}

	return nActivity	
}

func CheckCaptcha(db *sql.DB, captcha string) bool {
	parts := strings.Split(captcha, ":")

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[1], " ") == ""{
		return false
	}
	
	path  := "public/" + parts[0] + ".png"
	code  := GetCaptchaCodeDB(db, path)

	if code != "" {
		DeleteCaptchaCodeDB(db, path)
		CreateNewCaptcha(db)
	}

	if (code == strings.ToUpper(parts[1])) {
		return true
	}

	return false
}

func ParseInboxRequest(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	activity := GetActivityFromJson(r, db)
	if !VerifyHeaderSignature(r, *activity.Actor) {
		response := RejectActivity(activity)
		MakeActivityRequest(db, response)				
		return
	}

	switch(activity.Type) {
	case "Create":
		for _, e := range activity.To {
			if IsActorLocal(db, e) {
				if !IsActorLocal(db, activity.Actor.Id) {
					WriteObjectToCache(db, *activity.Object)
				}
			}
		}
		break

	case "Delete":
		for _, e := range activity.To {
			actor := GetActorFromDB(db, e)
			if actor.Id != "" {
				if activity.Object.Replies != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						TombstoneObject(db, k.Id)					
					}
				}
				TombstoneObject(db, activity.Object.Id)
				break
			}
		}
		break

		
	case "Follow":
		for _, e := range activity.To {
			if GetActorFromDB(db, e).Id != "" {
				response := AcceptFollow(activity)
				response = SetActorFollowerDB(db, response)
				MakeActivityRequest(db, response)
			} else {
				fmt.Println("follow request for rejected")				
				response := RejectActivity(activity)
				MakeActivityRequest(db, response)
				return
			}
		}
		break

	case "Reject":
		if activity.Object.Object.Type == "Follow" {
			fmt.Println("follow rejected")									
			SetActorFollowingDB(db, activity)
		}
		break		
	}

}

func MakeActivityFollowingReq(w http.ResponseWriter, r *http.Request, activity Activity) bool {
	actor := GetActor(activity.Object.Id)
	
	resp, err := http.NewRequest("POST", actor.Inbox, nil)

	CheckError(err, "Cannot make new get request to actor inbox for following req")

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var respActivity Activity

	err = json.Unmarshal(body, &respActivity)

	if respActivity.Type == "Accept" {
		return true
	}

	return false
}

func RemoteActorHasAuth(actor string, code string) bool {

	if actor == "" || code == "" {
		return false
	}
	
	req, err := http.NewRequest("GET", actor + "/verification&code=" + code, nil)

	CheckError(err, "could not make remote actor auth req")

	resp, err := http.DefaultClient.Do(req)

	CheckError(err, "could not make remote actor auth resp")

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true
	}

	return false
}
