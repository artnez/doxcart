package dropboxWrite

import (
  "bytes"
  "net/url"
  "net/http"
  "io/ioutil"
  "encoding/json"
  "appengine"
  "appengine/user"
  "appengine/urlfetch"
  "appengine/datastore"
  "web"
  "cache"
  "config"
	"dropbox/common"
)

const dropboxEndpoint = "https://www.dropbox.com/1/oauth2/authorize"
const dropboxCallbackDev = "http://localhost:8080/_/dropbox/oauth"
const dropboxCallback = "https://doxcart.appspot.com/_/dropbox/oauth"
const dropboxTokenEndpoint = "https://api.dropbox.com/1/oauth2/token"

func Init() {
  http.HandleFunc("/_/dropbox", setupHandler)
  http.HandleFunc("/_/dropbox/oauth", oauthHandler)
  http.HandleFunc("/_/dropbox/disconnect", disconnectHandler)
}

func getCallbackUrl() string {
  if appengine.IsDevAppServer() {
    return dropboxCallbackDev
  } else {
    return dropboxCallback
  }
}

func getLoginUrl() string {
  v := url.Values{}
  v.Set("response_type", "code")
  v.Set("client_id", config.DropboxAppKey)
  v.Set("redirect_uri", getCallbackUrl())
  u, _ := url.Parse(dropboxEndpoint)
  u.RawQuery = v.Encode()
  return u.String()
}

func setupHandler(w http.ResponseWriter, r *http.Request) {
  u := getLoginUrl()
  c := appengine.NewContext(r)
  c.Infof("Redirecting to %v", u)
  http.Redirect(w, r, u, http.StatusFound)
}

func oauthHandler(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c, _, done := web.Auth(c, w, r);
  if done == true { return }

  r.ParseForm()
  v := url.Values{}
  v.Set("code", r.Form["code"][0])
  v.Set("grant_type", "authorization_code")
  v.Set("client_id", config.DropboxAppKey)
  v.Set("client_secret", config.DropboxAppSecret)
  v.Set("redirect_uri", getCallbackUrl())

  client := urlfetch.Client(c)
  resp, err := client.Post(dropboxTokenEndpoint, "application/x-www-form-urlencoded", bytes.NewBufferString(v.Encode()))
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }
  c.Infof("HTTP POST returned %v", string(body))
  response := make(map[string]string)
  err = json.Unmarshal(body, &response)
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }

  err = dropboxCommon.SetToken(c, user.Current(c).Email, response["access_token"], response["uid"], "")
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }
  http.Redirect(w, r, "/", http.StatusFound)
}

func disconnectHandler(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  c, _, done := web.Auth(c, w, r);
  if done == true { return }

  domain := web.GetDomain(c)
  token, err := dropboxCommon.GetToken(c, domain)
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }

  if token == "" {
    http.Redirect(w, r, "/", http.StatusFound)
  }

  err = clearToken(c, token)
  if err != nil {
    web.ErrorPage(c, w, err)
    return
  }
  http.Redirect(w, r, "/", http.StatusFound)
}

func clearToken(c appengine.Context, token string) error {
  u := "https://api.dropbox.com/1/disable_access_token"
  c.Infof("Posting to %v", u)
  req, err := http.NewRequest("POST", u, bytes.NewBufferString(""))
  if err != nil { return err }
  req.Header.Set("Authorization", "Bearer " + token)
  client := urlfetch.Client(c)
  resp, err := client.Do(req)
  if err != nil { return err }
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil { return err }
  c.Infof("Got %v", string(body))

  domain := web.GetDomain(c)
  err = cache.Clear(c, "dropbox:" + domain)
  if err != nil { return err }
  err = datastore.Delete(c, datastore.NewKey(c, "ServiceToken", web.GetDomain(c) + "/dropbox", 0, nil))
  if err == datastore.ErrNoSuchEntity { return nil }
  return err
}

func SaveFile(c appengine.Context, domain string, path string, content string) error {
  token, err := dropboxCommon.GetToken(c, domain)
  if err != nil { return err }
  if token == "" { return nil }

  if path == "/" { path = "/home" }

  u := "https://api-content.dropbox.com/1/files_put/dropbox" + dropboxCommon.PathPrefix + path + ".md"
  c.Infof("Putting to %v", u)
  req, err := http.NewRequest("PUT", u, bytes.NewBufferString(content))
  if err != nil { return err }
  req.Header.Set("Authorization", "Bearer " + token)
  client := urlfetch.Client(c)

  if err != nil { return err }
  resp, err := client.Do(req)
  if err != nil { return err }
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil { return err }
  c.Infof("Got %v", string(body))
  return nil
}

