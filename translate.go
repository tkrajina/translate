package translate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	GrantType    string
	ScopeUrl     string
	ClientId     string
	ClientSecret string
	AuthUrl      string
}

type Token struct {
	AccessToken string    `json:"access_token"`
	Timestamp   time.Time `json:"-"`
	ExpiresIn   string    `json:"expires_in"`
}

func GetToken(c *Config) (token *Token, err error) {
	return GetTokenWithClient(&http.Client{}, c)
}
func GetTokenWithClient(client *http.Client, c *Config) (token *Token, err error) {
	values := make(url.Values)
	values.Set("grant_type", c.GrantType)
	values.Set("scope", c.ScopeUrl)
	values.Set("client_id", c.ClientId)
	values.Set("client_secret", c.ClientSecret)

	resp, err := client.PostForm(c.AuthUrl, values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll((*resp).Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, errors.New((*resp).Status + ":" + string(respBody))
	}
	json.Unmarshal(respBody, &token)
	token.Timestamp = time.Now()
	return
}

func (token *Token) Translate(text, from, to string) (result string, err error) {
	return token.TranslateWithClient(&http.Client{}, text, from, to)
}
func (token *Token) TranslateWithClient(client *http.Client, text, from, to string) (result string, err error) {
	window, err := time.ParseDuration(token.ExpiresIn + "s")
	if err != nil {
		return "", err
	}
	if token.Timestamp.Add(window).Before(time.Now()) {
		return "", errors.New("Access token expired")
	}
	if text == "" {
		return "", errors.New("\"text\" is a required parameter")
	}
	if to == "" {
		return "", errors.New("\"to\" is a required parameter")
	}
	params := "from=" + from + "&to=" + to + "&text=" + url.QueryEscape(text)
	uri := "http://api.microsofttranslator.com/v2/Http.svc/Translate?" + params
	req, err := http.NewRequest("GET", uri, nil)
	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	req.Header.Add("Content-Type", "text/plain")
	resp, err := client.Do(req)
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll((*resp).Body)
	err = xml.Unmarshal(bytes, &result)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", errors.New((*resp).Status + ":" + string(bytes))
	}
	return
}

func (token *Token) TranslateArray(texts []string, from, to string) (result []string, err error) {
	return token.TranslateArrayWithClient(&http.Client{}, texts, from, to)
}
func (token *Token) TranslateArrayWithClient(client *http.Client, texts []string, from, to string) (result []string, err error) {
	window, err := time.ParseDuration(string(token.ExpiresIn) + "s")
	if err != nil {
		return nil, err
	}
	if token.Timestamp.Add(window).Before(token.Timestamp.UTC()) {
		return nil, errors.New("Access token expired")
	}
	if texts == nil {
		return nil, errors.New("\"texts\" is a required parameter")
	}
	if to == "" {
		return nil, errors.New("\"to\" is a required parameter")
	}

	type MSString struct {
		XMLName     xml.Name `xml:"string"`
		String      string   `xml:",chardata"`
		StringXMLNS string   `xml:"xmlns,attr"`
	}

	type Request struct {
		XMLName xml.Name `xml:"TranslateArrayRequest"`
		AppId   string
		From    string
		Texts   []MSString `xml:"Texts>string"`
		Options string
		To      string
	}

	msStrings := []MSString{}
	for _, text := range texts {
		msStrings = append(msStrings, MSString{String: text, StringXMLNS: "http://schemas.microsoft.com/2003/10/Serialization/Arrays"})
	}

	data, err := xml.MarshalIndent(&Request{From: from, To: to, Texts: msStrings}, "", "  ")
	if err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(data)

	uri := "http://api.microsofttranslator.com/v2/Http.svc/TranslateArray"
	req, err := http.NewRequest("POST", uri, body)
	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	req.Header.Add("Content-Type", "text/xml")
	resp, err := client.Do(req)
	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll((*resp).Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, errors.New((*resp).Status)
	}

	type TranslateArrayResponse struct {
		Error                     string
		OriginalSentenceLengths   []int
		TranslatedText            string
		TranslatedSentenceLengths []int
		State                     string
	}
	type Response struct {
		XMLName   xml.Name                 `xml:"ArrayOfTranslateArrayResponse"`
		Responses []TranslateArrayResponse `xml:"TranslateArrayResponse"`
	}

	response := Response{}
	err = xml.Unmarshal(respBody, &response)
	if err != nil {
		return nil, err
	}

	for _, response := range response.Responses {
		result = append(result, response.TranslatedText)
	}

	return
}
