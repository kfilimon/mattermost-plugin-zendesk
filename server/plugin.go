package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
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
}

const (
	routeOAuthComplete = "/oauth/complete"
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
	//p.API.LogDebug("OK: ", "Status", strconv.Itoa(status), "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method, "query", r.URL.Query().Encode())
}

func handleHTTPRequest(p *Plugin, w http.ResponseWriter, r *http.Request) (int, error) {
	switch r.URL.Path {
	case routeUserConnect:
		return httpUserConnect(w, r)
	case routeTest:
		return handleTest(w, r)
	}

	return http.StatusNotFound, errors.New("not found")
}

func handleTest(w http.ResponseWriter, r *http.Request) (int, error) {
	fmt.Fprint(w, "Hello, world!")
	return http.StatusOK, nil
}

func httpUserConnect(w http.ResponseWriter, r *http.Request) (int, error) {
	if r.Method != http.MethodGet {
		return http.StatusMethodNotAllowed,
			errors.New("method " + r.Method + " is not allowed, must be GET")
	}

	mattermostUserID := r.Header.Get("Mattermost-User-Id")
	if mattermostUserID == "" {
		return http.StatusUnauthorized, errors.New("not authorized")
	}

	redirectURL := "https://google.com"

	http.Redirect(w, r, redirectURL, http.StatusFound)
	return http.StatusFound, nil
}

// GetPluginURLPath -
func (p *Plugin) GetPluginURLPath() string {
	return "/plugins/" + manifest.Id
}

// GetPluginURL -
func (p *Plugin) GetPluginURL() string {
	return strings.TrimRight(p.GetSiteURL(), "/") + p.GetPluginURLPath()
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

	client, err := zendesk.NewClient("my-testhelp", username, password)
	if err != nil {
		return errors.Wrap(err, "couldn't connect to zendesk")
	}
	p.zendeskClient = client

	return nil
}
