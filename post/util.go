package post

import (
	"fmt"
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
	"github.com/FChannel0/FChannel-Server/webfinger"
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
			return nil, err
		}

		if !util.IsInStringArray(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []activitypub.ObjectBase
	for i := 0; i < len(links); i++ {
		_, isValid, err := webfinger.CheckValidActivity(links[i])
		if err != nil {
			return nil, err
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
		_, isValid, err := webfinger.CheckValidActivity(strings.ReplaceAll(links[0], ">", ""))
		if err != nil {
			return "", err
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
	code, err := db.GetCaptchaCodeDB(path)
	if err != nil {
		return false, err
	}

	if code != "" {
		err = db.DeleteCaptchaCodeDB(path)
		if err != nil {
			return false, err
		}

		err = db.CreateNewCaptcha()
		if err != nil {
			return false, err
		}

	}

	return code == strings.ToUpper(parts[1]), nil
}

func IsMediaBanned(f multipart.File) (bool, error) {
	f.Seek(0, 0)

	fileBytes := make([]byte, 2048)

	_, err := f.Read(fileBytes)
	if err != nil {
		return true, err
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

	header, _ := ctx.FormFile("file")

	var file multipart.File

	if header != nil {
		file, _ = header.Open()
	}

	var err error

	if file != nil {
		defer file.Close()

		var tempFile = new(os.File)
		obj.Attachment, tempFile, err = activitypub.CreateAttachmentObject(file, header)
		if err != nil {
			return obj, err
		}

		defer tempFile.Close()

		fileBytes, _ := ioutil.ReadAll(file)

		tempFile.Write(fileBytes)

		re := regexp.MustCompile(`image/(jpe?g|png|webp)`)
		if re.MatchString(obj.Attachment[0].MediaType) {
			fileLoc := strings.ReplaceAll(obj.Attachment[0].Href, config.Domain, "")

			cmd := exec.Command("exiv2", "rm", "."+fileLoc)

			if err := cmd.Run(); err != nil {
				return obj, err
			}
		}

		obj.Preview = activitypub.CreatePreviewObject(obj.Attachment[0])
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
		if local, _ := activitypub.IsActivityLocal(activity); !local {
			actor, err := webfinger.FingerActor(originalPost.Id)
			if err != nil {
				return obj, err
			}

			if !util.IsInStringArray(obj.To, actor.Id) {
				obj.To = append(obj.To, actor.Id)
			}
		} else if err != nil {
			return obj, err
		}
	}

	replyingTo, err := ParseCommentForReplies(ctx.FormValue("comment"), originalPost.Id)

	if err != nil {
		return obj, err
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

			if local, err := activitypub.IsActivityLocal(activity); err == nil && !local {
				actor, err := webfinger.FingerActor(e.Id)
				if err != nil {
					return obj, err
				}

				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			} else if err != nil {
				return obj, err
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
				return err
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
					fmt.Println(objFile + " -> " + nHref)
					if err := activitypub.WritePreviewToDB(nPreview); err != nil {
						return err
					}
					if err := activitypub.UpdateObjectWithPreview(id, nPreview.Id); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}

		return nil
	})
}
