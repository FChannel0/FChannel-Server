package main

type Webfinger struct {
	Subject string `json:"subject,omitempty"`
	Links []WebfingerLink `json:"links,omitempty"`
}

type WebfingerLink struct {
	Rel string `json:"rel,omitempty"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`	
}
