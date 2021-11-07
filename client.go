package main

import (
	"html/template"
	"regexp"
	"strings"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/config"
	"github.com/FChannel0/FChannel-Server/db"
	"github.com/FChannel0/FChannel-Server/util"
	"github.com/FChannel0/FChannel-Server/webfinger"
	_ "github.com/lib/pq"
)

func MediaProxy(url string) string {
	re := regexp.MustCompile("(.+)?" + config.Domain + "(.+)?")

	if re.MatchString(url) {
		return url
	}

	re = regexp.MustCompile("(.+)?\\.onion(.+)?")

	if re.MatchString(url) {
		return url
	}

	MediaHashs[HashMedia(url)] = url
	return "/api/media?hash=" + HashMedia(url)
}

func ParseAttachment(obj activitypub.ObjectBase, catalog bool) template.HTML {
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
			media += "src=\"" + MediaProxy(obj.Preview.Href) + "\""
			media += "preview=\"" + MediaProxy(obj.Preview.Href) + "\""
		} else {
			media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\""
			media += "preview=\"" + MediaProxy(obj.Attachment[0].Href) + "\""
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
		media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\" "
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
		media += "src=\"" + MediaProxy(obj.Attachment[0].Href) + "\" "
		media += "type=\"" + obj.Attachment[0].MediaType + "\" "
		media += ">"
		media += "Video is not supported."
		media += "</video>"

		return template.HTML(media)
	}

	return template.HTML(media)
}

func ParseContent(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase) (template.HTML, error) {

	nContent := strings.ReplaceAll(content, `<`, "&lt;")

	nContent, err := ParseLinkComments(board, op, nContent, thread)
	if err != nil {
		return "", err
	}

	nContent = ParseCommentQuotes(nContent)

	nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

	return template.HTML(nContent), nil
}

func ParseLinkComments(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	//add url to each matched reply
	for i, _ := range match {
		link := strings.Replace(match[i][0], ">>", "", 1)
		isOP := ""

		domain := match[i][2]

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
				obj, err := db.GetObjectFromDBFromID(parsedLink)
				if err != nil {
					return "", err
				}

				if len(obj.OrderedItems) > 0 {
					quoteTitle = ParseLinkTitle(board.Outbox, op, obj.OrderedItems[0].Content)
				} else {
					quoteTitle = ParseLinkTitle(board.Outbox, op, parsedLink)
				}
			}
		}

		//replace link with quote format
		replyID, isReply, err := db.IsReplyToOP(op, parsedLink)
		if err != nil {
			return "", err
		}

		if isReply {
			id := util.ShortURL(board.Outbox, replyID)

			content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\"/"+board.Name+"/"+util.ShortURL(board.Outbox, op)+"#"+id+"\">&gt;&gt;"+id+""+isOP+"</a>", -1)

		} else {
			//this is a cross post

			parsedOP, err := db.GetReplyOP(parsedLink)
			if err != nil {
				return "", err
			}

			actor, err := webfinger.FingerActor(parsedLink)
			if err != nil {
				return "", err
			}

			if parsedOP != "" {
				link = parsedOP + "#" + util.ShortURL(parsedOP, parsedLink)
			}

			if actor.Id != "" {
				content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\""+link+"\">&gt;&gt;"+util.ShortURL(board.Outbox, parsedLink)+isOP+" →</a>", -1)
			}
		}
	}

	return content, nil
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

	content = strings.ReplaceAll(content, "'", "")
	content = strings.ReplaceAll(content, "\"", "")
	content = strings.ReplaceAll(content, ">", `/\&lt;`)

	return content
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
