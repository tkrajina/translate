package translate

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const bingSpeechTokenEndpoint = "https://api.cognitive.microsoft.com/sts/v1.0/issueToken"

type Token struct {
	AccessToken string `json:"access_token"`

	timestamp         time.Time
	reloadMutex       sync.Mutex
	expiresInDuration time.Duration
}

func GetTokenWithClient(client *http.Client, key string) (*Token, error) {
	req, err := http.NewRequest("POST", bingSpeechTokenEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Ocp-Apim-Subscription-Key", key)
	req.Header.Add("Content-Length", "0")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}

	defer res.Body.Close()
	size, err := strconv.Atoi(res.Header.Get("Content-Length"))
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	res.Body.Read(buf)
	return &Token{AccessToken: string(buf)}, nil
}

func GetToken(key string) (*Token, error) {
	client := &http.Client{}
	return GetTokenWithClient(client, key)
}

func (token Token) IsValid() bool {
	return token.expiresInDuration > 0 && time.Since(token.timestamp) < token.expiresInDuration
}

func (token *Token) RefreshIfNeeded(client *http.Client) error {
	return nil
}

func (token *Token) Translate(text, from, to string) (result string, err error) {
	return token.TranslateWithClient(&http.Client{}, text, from, to)
}
func (token *Token) TranslateWithClient(client *http.Client, text, from, to string) (result string, err error) {
	if err := token.RefreshIfNeeded(client); err != nil {
		return "", err
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
	if err := token.RefreshIfNeeded(client); err != nil {
		return nil, err
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
		return nil, errors.New((*resp).Status + ":" + string(respBody))
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
