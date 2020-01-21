package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/kfilimon/go-zendesk/zendesk"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
	"github.com/pkg/errors"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	// BotId of the created bot account.
	botID string

	// zendesk client
	zendeskClient zendesk.Client

	// map of the mattermost user with access token from zendesk
	oauthAccessTokenMap map[string]string

	zendeskURL           string
	zendeskClientSecrete string
}

const (
	routeOAuthRedirect = "/oauth/redirect"
	routeUserConnect   = "/user/connect"
	routeTest          = "/test"
)

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	status, err := handleHTTPRequest(p, w, r)
	if err != nil {
		p.API.LogError("ERROR: ", "Status", strconv.Itoa(status), "Error", err.Error(), "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method, "query", r.URL.Query().Encode())
		http.Error(w, err.Error(), status)
		return
	}
	switch status {
	case http.StatusOK:
		// pass through
	case 0:
		status = http.StatusOK
	default:
		w.WriteHeader(status)
	}
	p.API.LogDebug("OK: ", "Status", strconv.Itoa(status), "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method, "query", r.URL.Query().Encode())
}

func handleHTTPRequest(p *Plugin, w http.ResponseWriter, r *http.Request) (int, error) {
	switch r.URL.Path {
	case routeUserConnect:
		return httpUserConnect(p, w, r)
	case routeOAuthRedirect:
		return httpOAuthRedirect(p, w, r)
	case routeTest:
		return handleTest(w, r)
	}

	return http.StatusNotFound, errors.New("not found")
}

func handleTest(w http.ResponseWriter, r *http.Request) (int, error) {
	fmt.Fprint(w, "Hello, world!")
	return http.StatusOK, nil
}

func httpUserConnect(p *Plugin, w http.ResponseWriter, r *http.Request) (int, error) {
	// if access token is already associated with the muser then it means connection is not required
	// and we might skip going here; on the other hand if access token is revoked then how we would know
	// that it's expired, so we do need to come here; TODO: research

	if r.Method != http.MethodGet {
		return http.StatusMethodNotAllowed,
			errors.New("method " + r.Method + " is not allowed, must be GET")
	}

	zendeskURL := p.getConfiguration().ZendeskURL
	pluginURL := p.GetPluginURL()

	redirectURL := zendeskURL + "/oauth/authorizations/new?" +
		"response_type=code&" +
		"redirect_uri=" + pluginURL + "/oauth/redirect&" +
		"client_id=mattermost_integration_for_zendesk&" +
		"scope=read%20write"
	p.API.LogDebug("zendeskplugin: redirecturl:" + redirectURL)

	http.Redirect(w, r, redirectURL, http.StatusFound)
	return http.StatusFound, nil
}

// OAuthAccessResponse -
type OAuthAccessResponse struct {
	AccessToken string `json:"access_token"`
}

// OAuthAccessRequest -
type OAuthAccessRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_uri"`
	Scope        string `json:"scope"`
}

func httpOAuthRedirect(p *Plugin, w http.ResponseWriter, r *http.Request) (int, error) {

	// if there is "error" in the query string then it means user didn't authorize mattermost to go to zendesk
	// so we need to check for error and show it for muser

	// if the redirect url doesn't contain error - happy path:

	// get the value of the `code` query param
	err := r.ParseForm()
	if err != nil {
		fmt.Fprint(w, "Something went wrong: "+err.Error())
		return http.StatusOK, nil
	}
	code := r.FormValue("code")

	// Call the zendesk oauth endpoint to get access token
	reqURL := p.configuration.ZendeskURL + "/oauth/tokens"

	clientID := p.getConfiguration().ZendeskClientID
	clientSecret := p.getConfiguration().ZendeskClientSecrete

	redirectURL := p.GetPluginURL() + "/oauth/redirect"
	oauthRequest := OAuthAccessRequest{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scope:        "read write",
	}

	requestBodyBytes, err := json.Marshal(oauthRequest)
	if err != nil {
		fmt.Fprint(w, "Something went wrong: "+err.Error())
		return http.StatusOK, nil
	}
	requestBody := requestBodyBytes
	p.API.LogDebug(string(requestBody))

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewBuffer([]byte(requestBody)))
	if err != nil {
		fmt.Fprint(w, "Something went wrong: "+err.Error())
		return http.StatusOK, nil
	}
	req.Header.Set("Content-Type", "application/json")

	// Send out the HTTP request
	httpClient := http.Client{}
	res, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprint(w, "Something went wrong: "+err.Error())
		return http.StatusOK, nil
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 400 {
		bodyBytes, _ := ioutil.ReadAll(res.Body)
		bodyString := string(bodyBytes)
		fmt.Fprint(w, "Could not obtain OAuth access token from zendesk: "+bodyString)
		return http.StatusOK, nil
	}

	// Parse the response with access token
	var oauthResponse OAuthAccessResponse
	if err = json.NewDecoder(res.Body).Decode(&oauthResponse); err != nil {
		fmt.Fprint(w, "Something went wrong: "+err.Error())
		return http.StatusOK, nil
	}

	mattermostUserID := r.Header.Get("Mattermost-User-ID")
	//TODO: how to get UserName
	p.oauthAccessTokenMap[mattermostUserID] = oauthResponse.AccessToken

	fmt.Fprint(w, "Successfully connected mattermost account "+
		mattermostUserID+" "+
		" with zendesk account: "+oauthResponse.AccessToken)

	return http.StatusOK, nil
}

// GetPluginURLPath -
func (p *Plugin) GetPluginURLPath() string {
	return "/plugins/" + manifest.Id
}

// GetPluginURL -
func (p *Plugin) GetPluginURL() string {
	siteURL := p.GetSiteURL()

	// workaround for localhost testing
	if !strings.Contains(siteURL, ".com") {
		siteURL = "http://localhost:8066"
	}

	return strings.TrimRight(siteURL, "/") + p.GetPluginURLPath()
}

// GetSiteURL -
func (p *Plugin) GetSiteURL() string {
	ptr := p.API.GetConfig().ServiceSettings.SiteURL
	if ptr == nil {
		return ""
	}
	return *ptr
}

// OnActivate -
func (p *Plugin) OnActivate() error {
	// register commands
	err := p.API.RegisterCommand(getCommand())
	if err != nil {
		return errors.WithMessage(err, "OnActivate: failed to register command")
	}

	p.oauthAccessTokenMap = make(map[string]string)

	// ensure bot
	botID, ensureBotError := p.Helpers.EnsureBot(&model.Bot{
		Username:    "zendesk",
		DisplayName: "Zendesk Bot",
		Description: "A bot account created by the zendesk plugin.",
	})

	if ensureBotError != nil {
		return errors.Wrap(ensureBotError, "failed to ensure zendesk bot.")
	}
	p.botID = botID

	// set profile image for the bot
	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return errors.Wrap(err, "couldn't get bundle path")
	}
	profileImage, err := ioutil.ReadFile(filepath.Join(bundlePath, "assets", "zendesklogo.png"))
	if err != nil {
		return errors.Wrap(err, "couldn't read profile image")
	}
	if appErr := p.API.SetProfileImage(p.botID, profileImage); appErr != nil {
		return errors.Wrap(appErr, "couldn't set profile image")
	}

	// connect to zendesk
	username := os.Getenv("ZENDESK_USER")
	password := os.Getenv("ZENDESK_PASSWORD")

	u, _ := url.Parse(p.getConfiguration().ZendeskURL)
	clientHost := strings.Split(u.Host, ".")[0]

	client, err := zendesk.NewClient(clientHost, username, password)
	if err != nil {
		return errors.Wrap(err, "couldn't connect to zendesk")
	}
	p.zendeskClient = client

	return nil
}
