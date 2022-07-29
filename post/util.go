package post

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"mime/multipart"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/gofiber/fiber/v2"
)

func ConvertHashLink(domain string, link string) string {
	re := regexp.MustCompile(`(#.+)`)
	parsedLink := re.FindString(link)

	if parsedLink != "" {
		parsedLink = domain + "" + strings.Replace(parsedLink, "#", "", 1)
		parsedLink = strings.Replace(parsedLink, "\r", "", -1)
	} else {
		parsedLink = link
	}

	return parsedLink
}

func ParseCommentForReplies(comment string, op string) ([]activitypub.ObjectBase, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)
		str = config.TP + "" + str
		_, isReply, err := db.IsReplyToOP(op, str)

		if err != nil {
			return nil, util.MakeError(err, "ParseCommentForReplies")
		}

		if !util.IsInStringArray(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []activitypub.ObjectBase
	for i := 0; i < len(links); i++ {
		reqActivity := activitypub.Activity{Id: links[i]}
		_, isValid, err := reqActivity.CheckValid()

		if err != nil {
			return nil, util.MakeError(err, "ParseCommentForReplies")
		}

		if isValid {
			var reply activitypub.ObjectBase

			reply.Id = links[i]
			reply.Published = time.Now().UTC()
			validLinks = append(validLinks, reply)
		}
	}

	return validLinks, nil
}

func ParseCommentForReply(comment string) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		links = append(links, str)
	}

	if len(links) > 0 {
		reqActivity := activitypub.Activity{Id: strings.ReplaceAll(links[0], ">", "")}
		_, isValid, err := reqActivity.CheckValid()

		if err != nil {
			return "", util.MakeError(err, "ParseCommentForReply")
		}

		if isValid {
			return links[0], nil
		}
	}

	return "", nil
}

func ParseLinkTitle(actorName string, op string, content string) string {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)\w+(#.+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	for i, _ := range match {
		link := strings.Replace(match[i][0], ">>", "", 1)
		isOP := ""

		domain := match[i][2]

		if link == op {
			isOP = " (OP)"
		}

		link = ConvertHashLink(domain, link)
		content = strings.Replace(content, match[i][0], ">>"+util.ShortURL(actorName, link)+isOP, 1)
	}

	content = strings.ReplaceAll(content, "'", "&#39;")
	content = strings.ReplaceAll(content, "\"", "&quot;")
	content = strings.ReplaceAll(content, ">", `/\&lt;`)

	return content
}

func ParseOptions(ctx *fiber.Ctx, obj activitypub.ObjectBase) activitypub.ObjectBase {
	options := util.EscapeString(ctx.FormValue("options"))

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
				obj.Option = append(obj.Option, "email:"+e)
			} else if wallet.MatchString(e) {
				obj.Option = append(obj.Option, "wallet")
				var wallet activitypub.CryptoCur
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

func CheckCaptcha(captcha string) (bool, error) {
	parts := strings.Split(captcha, ":")

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[1], " ") == "" {
		return false, nil
	}

	path := "public/" + parts[0] + ".png"
	code, err := util.GetCaptchaCode(path)

	if err != nil {
		return false, util.MakeError(err, "ParseOptions")
	}

	if code != "" {
		err = util.DeleteCaptchaCode(path)
		if err != nil {
			return false, util.MakeError(err, "ParseOptions")
		}

		err = util.CreateNewCaptcha()
		if err != nil {
			return false, util.MakeError(err, "ParseOptions")
		}

	}

	return code == strings.ToUpper(parts[1]), nil
}

func GetCaptchaCode(captcha string) string {
	re := regexp.MustCompile("\\w+\\.\\w+$")
	code := re.FindString(captcha)

	re = regexp.MustCompile("\\w+")
	code = re.FindString(code)

	return code
}

func IsMediaBanned(f multipart.File) (bool, error) {
	f.Seek(0, 0)
	fileBytes := make([]byte, 2048)
	_, err := f.Read(fileBytes)

	if err != nil {
		return true, util.MakeError(err, "IsMediaBanned")
	}

	hash := util.HashBytes(fileBytes)
	f.Seek(0, 0)

	return db.IsHashBanned(hash)
}

func SupportedMIMEType(mime string) bool {
	for _, e := range config.SupportedFiles {
		if e == mime {
			return true
		}
	}

	return false
}

func ObjectFromForm(ctx *fiber.Ctx, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	var err error
	var file multipart.File

	header, _ := ctx.FormFile("file")

	if header != nil {
		file, _ = header.Open()
	}

	if file != nil {
		defer file.Close()
		var tempFile = new(os.File)

		obj.Attachment, tempFile, err = activitypub.CreateAttachmentObject(file, header)

		if err != nil {
			return obj, util.MakeError(err, "ObjectFromForm")
		}

		defer tempFile.Close()

		fileBytes, _ := ioutil.ReadAll(file)
		tempFile.Write(fileBytes)

		re := regexp.MustCompile(`image/(jpe?g|png|webp)`)
		if re.MatchString(obj.Attachment[0].MediaType) {
			fileLoc := strings.ReplaceAll(obj.Attachment[0].Href, config.Domain, "")

			cmd := exec.Command("exiv2", "rm", "."+fileLoc)

			if err := cmd.Run(); err != nil {
				return obj, util.MakeError(err, "ObjectFromForm")
			}
		}

		obj.Preview = obj.Attachment[0].CreatePreview()
	}

	obj.AttributedTo = util.EscapeString(ctx.FormValue("name"))
	obj.TripCode = util.EscapeString(ctx.FormValue("tripcode"))
	obj.Name = util.EscapeString(ctx.FormValue("subject"))
	obj.Content = util.EscapeString(ctx.FormValue("comment"))
	obj.Sensitive = (ctx.FormValue("sensitive") != "")
	obj = ParseOptions(ctx, obj)

	var originalPost activitypub.ObjectBase

	originalPost.Id = util.EscapeString(ctx.FormValue("inReplyTo"))
	obj.InReplyTo = append(obj.InReplyTo, originalPost)

	var activity activitypub.Activity

	if !util.IsInStringArray(activity.To, originalPost.Id) {
		activity.To = append(activity.To, originalPost.Id)
	}

	if originalPost.Id != "" {
		if local, _ := activity.IsLocal(); !local {
			actor, err := activitypub.FingerActor(originalPost.Id)
			if err == nil { // Keep things moving if it fails
				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			}
		} else if err != nil {
			return obj, util.MakeError(err, "ObjectFromForm")
		}
	}

	replyingTo, err := ParseCommentForReplies(ctx.FormValue("comment"), originalPost.Id)

	if err != nil {
		return obj, util.MakeError(err, "ObjectFromForm")
	}

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

			var activity activitypub.Activity

			activity.To = append(activity.To, e.Id)

			if local, _ := activity.IsLocal(); !local {
				actor, err := activitypub.FingerActor(e.Id)
				if err != nil {
					return obj, util.MakeError(err, "ObjectFromForm")
				}

				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			}
		}
	}

	return obj, nil
}

func ResizeAttachmentToPreview() error {
	return activitypub.GetObjectsWithoutPreviewsCallback(func(id, href, mediatype, name string, size int, published time.Time) error {
		re := regexp.MustCompile(`^\w+`)
		_type := re.FindString(mediatype)

		if _type == "image" {
			re = regexp.MustCompile(`.+/`)
			file := re.ReplaceAllString(mediatype, "")
			nHref := util.GetUniqueFilename(file)

			var nPreview activitypub.NestedObjectBase

			re = regexp.MustCompile(`/\w+$`)
			actor := re.ReplaceAllString(id, "")
			nPreview.Type = "Preview"
			uid, err := util.CreateUniqueID(actor)

			if err != nil {
				return util.MakeError(err, "ResizeAttachmentToPreview")
			}

			nPreview.Id = fmt.Sprintf("%s/%s", actor, uid)
			nPreview.Name = name
			nPreview.Href = config.Domain + "" + nHref
			nPreview.MediaType = mediatype
			nPreview.Size = int64(size)
			nPreview.Published = published
			nPreview.Updated = published
			re = regexp.MustCompile(`/public/.+`)
			objFile := re.FindString(href)

			if id != "" {
				cmd := exec.Command("convert", "."+objFile, "-resize", "250x250>", "-strip", "."+nHref)

				if err := cmd.Run(); err == nil {
					config.Log.Println(objFile + " -> " + nHref)
					if err := nPreview.WritePreview(); err != nil {
						return util.MakeError(err, "ResizeAttachmentToPreview")
					}
					obj := activitypub.ObjectBase{Id: id}
					if err := obj.UpdatePreview(nPreview.Id); err != nil {
						return util.MakeError(err, "ResizeAttachmentToPreview")
					}
				} else {
					return util.MakeError(err, "ResizeAttachmentToPreview")
				}
			}
		}

		return nil
	})
}

func ParseAttachment(obj activitypub.ObjectBase, catalog bool) template.HTML {
	// TODO: convert all of these to Sprintf statements, or use strings.Builder or something, anything but this really
	// string concatenation is highly inefficient _especially_ when being used like this

	if len(obj.Attachment) < 1 {
		return ""
	}

	var media string

	if regexp.MustCompile(`image\/`).MatchString(obj.Attachment[0].MediaType) {
		media = "<img "
		media += "id=\"img\" "
		media += "main=\"1\" "
		media += "enlarge=\"0\" "
		media += "attachment=\"" + obj.Attachment[0].Href + "\""
		if catalog {
			media += "style=\"max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\""
		}
		if obj.Preview.Id != "" {
			media += "src=\"" + util.MediaProxy(obj.Preview.Href) + "\""
			media += "preview=\"" + util.MediaProxy(obj.Preview.Href) + "\""
		} else {
			media += "src=\"" + util.MediaProxy(obj.Attachment[0].Href) + "\""
			media += "preview=\"" + util.MediaProxy(obj.Attachment[0].Href) + "\""
		}

		media += ">"

		return template.HTML(media)
	}

	if regexp.MustCompile(`audio\/`).MatchString(obj.Attachment[0].MediaType) {
		media = "<audio "
		media += "controls=\"controls\" "
		media += "preload=\"metadta\" "
		if catalog {
			media += "style=\"margin-right: 10px; margin-bottom: 10px; max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\" "
		}
		media += ">"
		media += "<source "
		media += "src=\"" + util.MediaProxy(obj.Attachment[0].Href) + "\" "
		media += "type=\"" + obj.Attachment[0].MediaType + "\" "
		media += ">"
		media += "Audio is not supported."
		media += "</audio>"

		return template.HTML(media)
	}

	if regexp.MustCompile(`video\/`).MatchString(obj.Attachment[0].MediaType) {
		media = "<video "
		media += "controls=\"controls\" "
		media += "preload=\"metadta\" "
		media += "muted=\"muted\" "
		if catalog {
			media += "style=\"margin-right: 10px; margin-bottom: 10px; max-width: 180px; max-height: 180px;\" "
		} else {
			media += "style=\"float: left; margin-right: 10px; margin-bottom: 10px; max-width: 250px; max-height: 250px;\" "
		}
		media += ">"
		media += "<source "
		media += "src=\"" + util.MediaProxy(obj.Attachment[0].Href) + "\" "
		media += "type=\"" + obj.Attachment[0].MediaType + "\" "
		media += ">"
		media += "Video is not supported."
		media += "</video>"

		return template.HTML(media)
	}

	return template.HTML(media)
}

func ParseContent(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase, id string, _type string) (template.HTML, error) {
	// TODO: should escape more than just < and >, should also escape &, ", and '
	nContent := strings.ReplaceAll(content, `<`, "&lt;")

	if _type == "new" {
		nContent = ParseTruncate(nContent, board, op, id)
	}

	nContent, err := ParseLinkComments(board, op, nContent, thread)

	if err != nil {
		return "", util.MakeError(err, "ParseContent")
	}

	nContent = ParseCommentQuotes(nContent)
	nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

	return template.HTML(nContent), nil
}

func ParseTruncate(content string, board activitypub.Actor, op string, id string) string {
	if strings.Count(content, "\r") > 30 {
		content = strings.ReplaceAll(content, "\r\n", "\r")
		lines := strings.SplitAfter(content, "\r")
		content = ""

		for i := 0; i < 30; i++ {
			content += lines[i]
		}

		content += fmt.Sprintf("<a href=\"%s\">(view full post...)</a>", board.Id+"/"+util.ShortURL(board.Outbox, op)+"#"+util.ShortURL(board.Outbox, id))
	}

	return content
}

func ParseLinkComments(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	//add url to each matched reply
	for i, _ := range match {
		isOP := ""
		domain := match[i][2]
		link := strings.Replace(match[i][0], ">>", "", 1)

		if link == op {
			isOP = " (OP)"
		}

		parsedLink := ConvertHashLink(domain, link)

		//formate the hover title text
		var quoteTitle string

		// if the quoted content is local get it
		// else get it from the database
		if thread.Id == link {
			quoteTitle = ParseLinkTitle(board.Outbox, op, thread.Content)
		} else {
			for _, e := range thread.Replies.OrderedItems {
				if e.Id == parsedLink {
					quoteTitle = ParseLinkTitle(board.Outbox, op, e.Content)
					break
				}
			}

			if quoteTitle == "" {
				obj := activitypub.ObjectBase{Id: parsedLink}
				col, err := obj.GetCollectionFromPath()
				if err != nil {
					return "", util.MakeError(err, "ParseLinkComments")
				}

				if len(col.OrderedItems) > 0 {
					quoteTitle = ParseLinkTitle(board.Outbox, op, col.OrderedItems[0].Content)
				} else {
					quoteTitle = ParseLinkTitle(board.Outbox, op, parsedLink)
				}
			}
		}

		if replyID, isReply, err := db.IsReplyToOP(op, parsedLink); err == nil || !isReply {
			id := util.ShortURL(board.Outbox, replyID)

			content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\"/"+board.Name+"/"+util.ShortURL(board.Outbox, op)+"#"+id+"\">&gt;&gt;"+id+""+isOP+"</a>", -1)
		} else {
			//this is a cross post

			parsedOP, err := db.GetReplyOP(parsedLink)
			if err == nil {
				link = parsedOP + "#" + util.ShortURL(parsedOP, parsedLink)
			}

			actor, err := activitypub.FingerActor(parsedLink)
			if err == nil && actor.Id != "" {
				content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\""+link+"\">&gt;&gt;"+util.ShortURL(board.Outbox, parsedLink)+isOP+" →</a>", -1)
			}
		}
	}

	return content, nil
}

func ParseCommentQuotes(content string) string {
	// replace quotes
	re := regexp.MustCompile(`((\r\n|\r|\n|^)>(.+)?[^\r\n])`)
	match := re.FindAllStringSubmatch(content, -1)

	for i, _ := range match {
		quote := strings.Replace(match[i][0], ">", "&gt;", 1)
		line := re.ReplaceAllString(match[i][0], "<span class=\"quote\">"+quote+"</span>")
		content = strings.Replace(content, match[i][0], line, 1)
	}

	//replace isolated greater than symboles
	re = regexp.MustCompile(`(\r\n|\n|\r)>`)

	return re.ReplaceAllString(content, "\r\n<span class=\"quote\">&gt;</span>")
}
