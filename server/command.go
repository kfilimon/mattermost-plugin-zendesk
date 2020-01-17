package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

const helpTextHeader = "###### Mattermost Zendesk Plugin - Slash Command Help\n"

const commonHelpText = "\n* `/zendesk status <case-number>` - Retrieve the current status of a case\n" +
	"* `/zendesk update private <case-number>` - Post an Internal Comment to a case and notify agents\n"

// CommandHandlerFunc -
type CommandHandlerFunc func(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse

// CommandHandler -
type CommandHandler struct {
	handlers       map[string]CommandHandlerFunc
	defaultHandler CommandHandlerFunc
}

var zendeskCommandHandler = CommandHandler{
	handlers: map[string]CommandHandlerFunc{
		"status": executeStatus,
		"help":   commandHelp,
	},
	defaultHandler: executeZendeskDefault,
}

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "zendesk",
		DisplayName:      "Zendesk",
		Description:      "Integration with Zendesk.",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: status, update private, help",
		AutoCompleteHint: "[command]",
	}
}

// ExecuteCommand -
func (p *Plugin) ExecuteCommand(c *plugin.Context, commandArgs *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	args := strings.Fields(commandArgs.Command)
	if len(args) == 0 || args[0] != "/zendesk" {
		return p.help(commandArgs), nil
	}
	return zendeskCommandHandler.Handle(p, c, commandArgs, args[1:]...), nil
}

// Handle -
func (ch CommandHandler) Handle(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	for n := len(args); n > 0; n-- {
		h := ch.handlers[strings.Join(args[:n], "/")]
		if h != nil {
			return h(p, c, header, args[n:]...)
		}
	}
	return ch.defaultHandler(p, c, header, args...)
}

func commandHelp(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	return p.help(header)
}

func (p *Plugin) help(args *model.CommandArgs) *model.CommandResponse {
	helpText := helpTextHeader
	helpText += commonHelpText

	p.postCommandResponse(args, helpText)
	return &model.CommandResponse{}
}

// executeStatus returns the current status of a case, I.e. Pending, Open, On-Hold, Solved Closed
func executeStatus(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 1 {
		return p.responsef(header, "Please specify a case number in the form `/zendesk status <case-number>`.")
	}

	status := "On-Hold"

	p.postCommandResponse(header, status)
	return &model.CommandResponse{}
}

// executeZendeskDefault is the default command if no other command fits. It defaults to help.
func executeZendeskDefault(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	return p.help(header)
}

func (p *Plugin) postCommandResponse(args *model.CommandArgs, text string) {
	post := &model.Post{
		UserId:    p.botID,
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)
}

func (p *Plugin) responsef(commandArgs *model.CommandArgs, format string, args ...interface{}) *model.CommandResponse {
	p.postCommandResponse(commandArgs, fmt.Sprintf(format, args...))
	return &model.CommandResponse{}
}
