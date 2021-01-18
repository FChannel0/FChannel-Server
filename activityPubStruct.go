package main

import "encoding/json"

type AtContextRaw struct {
	Context json.RawMessage `json:"@context,omitempty"`
}

type ActivityRaw struct {
	AtContextRaw
	Type string `json:"type,omitempty"`
	Id string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`	
	Summary string `json:"summary,omitempty"`
	Auth string `json:"auth,omitempty"`		
	ToRaw json.RawMessage `json:"to,omitempty"`	
	BtoRaw json.RawMessage `json:"bto,omitempty"`	
	CcRaw json.RawMessage `json:"cc,omitempty"`	
	Published string `json:"published,omitempty"`
	ActorRaw json.RawMessage `json:"actor,omitempty"`
	ObjectRaw json.RawMessage `json:"object,omitempty"`	
}

type AtContext struct {
	Context string `json:"@context,omitempty"`
}

type AtContextArray struct {
	Context []interface {} `json:"@context,omitempty"`
}

type AtContextString struct {
	Context string `json:"@context,omitempty"`
}

type ActorString struct {
	Actor string `json:"actor,omitempty"`
}

type ObjectArray struct {
	Object []ObjectBase `json:"object,omitempty"`
}

type Object struct {
	Object *ObjectBase `json:"object,omitempty"`
}

type ObjectString struct {
	Object string `json:"object,omitempty"`
}

type ToArray struct {
	To []string `json:"to,omitempty"`
}

type ToString struct {
	To string `json:"to,omitempty"`
}

type CcArray struct {
	Cc []string `json:"cc,omitempty"`
}

type CcOjectString struct {
	Cc string `json:"cc,omitempty"`
}

type Actor struct {
	AtContext
	Type string `json:"type,omitempty"`
	Id string `json:"id,omitempty"`	
	Inbox string `json:"inbox,omitempty"`
	Outbox string `json:"outbox,omitempty"`
	Following string `json:"following,omitempty"`
	Followers string `json:"followers,omitempty"`
	Name string `json:"name,omitempty"`
	PreferredUsername string `json:"prefereedUsername,omitempty"`
	Summary string `json:"summary,omitempty"`
	AuthRequirement []string `json:"authrequirement,omitempty"`
	Restricted bool `json:"restricted,omitempty"`	
}

type Activity struct {
	AtContext
	Type string `json:"type,omitempty"`
	Id string `json:"id,omitempty"`
	Actor *Actor `json:"actor,omitempty"`		
	Name string `json:"name,omitempty"`	
	Summary string `json:"summary,omitempty"`
	Auth string `json:"auth,omitempty"`	
	To []string `json:"to, omitempty"`
	Bto []string `json:"bto,omitempty"`	
	Cc []string `json:"cc, omitempty"`
	Published string `json:"published,omitempty"`
	Object *ObjectBase `json:"object, omitempty"` 
}

type ObjectBase struct {
	AtContext
	Type string `json:"type,omitempty"`
	Id string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Option []string `json:"option,omitempty"`
	Alias string `json:"alias,omitempty"`	
	AttributedTo string `json:"attributedTo,omitempty"`
	Actor *Actor `json:"actor,omitempty"`
	Audience string `json:"audience,omitempty"`
	Content string `json:"content,omitempty"`
	EndTime string `json:"endTime,omitempty"`
	Generator string `json:"generator,omitempty"`
	Icon string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	InReplyTo []ObjectBase `json:"inReplyTo,omitempty"`
	Location string `json:"location,omitempty"`
	Preview *NestedObjectBase `json:"preview,omitempty"`		
	Published string `json:"published,omitempty"`
	Updated string `json:"updated,omitempty"`	
	Object *NestedObjectBase `json:"object,omitempty"`
	Attachment []ObjectBase `json:"attachment,omitempty"`
	Replies *CollectionBase `json:"replies,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	Summary string `json:"summary,omitempty"`
	Tag []ObjectBase `json:"tag,omitempty"`
	Wallet []CryptoCur `json:"wallet,omitempty"`	
	Deleted string `json:"deleted,omitempty"`	
	Url []ObjectBase `json:"url,omitempty"`
	Href string `json:"href,omitempty"`	
	To []string `json:"to,omitempty"`
	Bto []string `json:"bto,omitempty"`
	Cc []string `json:"cc,omitempty"`
	Bcc string `json:"Bcc,omitempty"`
	MediaType string `json:"mediatype,omitempty"`
	Duration string `json:"duration,omitempty"`
	Size int64 `json:"size,omitempty"`																							
}

type CryptoCur struct {
	Type string `json:"type,omitempty"`
	Address string `json:"address,omitempty"`
}

type NestedObjectBase struct {
	AtContext
	Type string `json:"type,omitempty"`
	Id string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Alias string `json:"alias,omitempty"`	
	AttributedTo string `json:"attributedTo,omitempty"`
	Actor *Actor `json:"actor,omitempty"`	
	Audience string `json:"audience,omitempty"`
	Content string `json:"content,omitempty"`
	EndTime string `json:"endTime,omitempty"`
	Generator string `json:"generator,omitempty"`
	Icon string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	InReplyTo []ObjectBase `json:"inReplyTo,omitempty"`
	Location string `json:"location,omitempty"`
	Preview ObjectBase `json:"preview,omitempty"`		
	Published string `json:"published,omitempty"`
	Attachment []ObjectBase `json:"attachment,omitempty"`
	Replies *CollectionBase `json:"replies,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	Summary string `json:"summary,omitempty"`
	Tag []ObjectBase `json:"tag,omitempty"`
	Updated string `json:"updated,omitempty"`
	Deleted string `json:"deleted,omitempty"`	
	Url []ObjectBase `json:"url,omitempty"`
	Href string `json:"href,omitempty"`	
	To []string `json:"to,omitempty"`
	Bto []string `json:"bto,omitempty"`
	Cc []string `json:"cc,omitempty"`
	Bcc string `json:"Bcc,omitempty"`
	MediaType string `json:"mediatype,omitempty"`
	Duration string `json:"duration,omitempty"`
	Size int64 `json:"size,omitempty"`																							
}

type CollectionBase struct {
	Actor string `json:"actor,omitempty"`
	Summary string `json:"summary,omitempty"`
	Type string `json:"type,omitempty"`
	TotalItems int `json:"totalItems,omitempty"`
	TotalImgs int `json:"totalImgs,omitempty"`
	OrderedItems []ObjectBase `json:"orderedItems,omitempty"`
	Items []ObjectBase `json:"items,omitempty"`		
}

type Collection struct {
	AtContext
	CollectionBase
}
