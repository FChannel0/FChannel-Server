package db

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"time"

	"github.com/FChannel0/FChannel-Server/activitypub"
	"github.com/FChannel0/FChannel-Server/webfinger"
	"github.com/gofiber/fiber/v2"
)

func VerifyHeaderSignature(ctx *fiber.Ctx, actor activitypub.Actor) bool {
	s := activitypub.ParseHeaderSignature(ctx.Get("Signature"))

	var method string
	var path string
	var host string
	var date string
	var digest string
	var contentLength string

	var sig string
	for i, e := range s.Headers {
		var nl string
		if i < len(s.Headers)-1 {
			nl = "\n"
		}

		switch e {
		case "(request-target)":
			method = strings.ToLower(ctx.Method())
			path = ctx.Path()
			sig += "(request-target): " + method + " " + path + "" + nl
			break
		case "host":
			host = ctx.Hostname()
			sig += "host: " + host + "" + nl
			break
		case "date":
			date = ctx.Get("date")
			sig += "date: " + date + "" + nl
			break
		case "digest":
			digest = ctx.Get("digest")
			sig += "digest: " + digest + "" + nl
			break
		case "content-length":
			contentLength = ctx.Get("content-length")
			sig += "content-length: " + contentLength + "" + nl
			break
		}
	}

	if s.KeyId != actor.PublicKey.Id {
		return false
	}

	t, _ := time.Parse(time.RFC1123, date)

	if time.Now().UTC().Sub(t).Seconds() > 75 {
		return false
	}

	if ActivityVerify(actor, s.Signature, sig) != nil {
		return false
	}

	return true
}

func ActivityVerify(actor activitypub.Actor, signature string, verify string) error {
	sig, _ := base64.StdEncoding.DecodeString(signature)

	if actor.PublicKey.PublicKeyPem == "" {
		_actor, err := webfinger.FingerActor(actor.Id)
		if err != nil {
			return err
		}
		actor = _actor
	}

	block, _ := pem.Decode([]byte(actor.PublicKey.PublicKeyPem))
	pub, _ := x509.ParsePKIXPublicKey(block.Bytes)

	hashed := sha256.New()
	hashed.Write([]byte(verify))

	return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hashed.Sum(nil), sig)
}
