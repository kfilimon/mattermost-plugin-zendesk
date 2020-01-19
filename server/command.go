package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kfilimon/go-zendesk/zendesk"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

const helpTextHeader = "###### Mattermost Zendesk Plugin - Slash Command Help\n"

const commonHelpText = "\n* `/zendesk status <case-number>` - Retrieve the current status of a case\n" +
	"* `/zendesk details <case-number>` - Return details of the case\n" +
	"* `/zendesk latest private <case-number>` - Retrieve the last internal comment posted to a case\n" +
	"* `/zendesk latest public <case-number>` - Retrieve the last public comment posted to a case\n" +
	"* `/zendesk update private <case-number>` - Post an internal comment to a case and notify agents\n" +
	"* `/zendesk update public <case-number>` - Post a public comment to a case and notify agents\n" +
	"* `/zendesk connect` - Connect to Zendesk\n" +
	"* `/zendesk help` - Show Help\n"

// CommandHandlerFunc -
type CommandHandlerFunc func(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse

// CommandHandler -
type CommandHandler struct {
	handlers       map[string]CommandHandlerFunc
	defaultHandler CommandHandlerFunc
}

var zendeskCommandHandler = CommandHandler{
	handlers: map[string]CommandHandlerFunc{
		"connect":        executeConnect,
		"status":         executeStatus,
		"latest/private": executeLatestPrivate,
		"latest/public":  executeLatestPublic,
		"update/private": executeUpdatePrivate,
		"update/public":  executeUpdatePublic,
		"details":        executeDetails,
		"help":           commandHelp,
	},
	defaultHandler: executeZendeskDefault,
}

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "zendesk",
		DisplayName:      "Zendesk",
		Description:      "Integration with Zendesk.",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: status, details, latest/private, latest/public, update/private, update/public, connect, help",
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

func executeConnect(p *Plugin, c *plugin.Context, header *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 0 {
		return p.help(header)
	}

	return p.responsef(header, "[Click here to link your Zendesk account](%s%s)",
		p.GetPluginURL(), routeUserConnect)
}

// executeStatus returns the current status of a case, I.e. Pending, Open, On-Hold, Solved Closed
func executeStatus(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 1 {
		return p.responsef(commandArgs, "Please specify a case number in the form `/zendesk status <case-number>`.")
	}

	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	ticket, err := p.zendeskClient.ShowTicket(ticketNumber)
	if err != nil {
		return p.responsef(commandArgs, err.Error())
	}

	status := *ticket.Status
	p.postCommandResponse(commandArgs, status)
	return &model.CommandResponse{}
}

// executeDetails - Return details of the case, Assignee, Requester, Organization, Issue, Priority, Status etc.
func executeDetails(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 1 {
		return p.responsef(commandArgs, "Please specify a case number in the form `/zendesk status <case-number>`.")
	}

	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	ticket, err := p.zendeskClient.ShowTicket(ticketNumber)
	if err != nil {
		return p.responsef(commandArgs, err.Error())
	}
	var organization *zendesk.Organization
	if ticket.OrganizationID != nil {
		organization, err = p.zendeskClient.ShowOrganization(*ticket.OrganizationID)
		if err != nil {
			return p.responsef(commandArgs, err.Error())
		}
	}

	attachment, err := p.parseTicket(ticket, organization)
	if err != nil {
		return p.responsef(commandArgs, err.Error())
	}

	post := &model.Post{
		UserId:    p.botID,
		ChannelId: commandArgs.ChannelId,
	}
	post.AddProp("attachments", attachment)

	_ = p.API.SendEphemeralPost(commandArgs.UserId, post)

	//TODO - remove - test only
	ticketStr, _ := json.Marshal(*ticket)
	p.postCommandResponse(commandArgs, "TEST ONLY - RAW OUTPUT: "+string(ticketStr))

	return &model.CommandResponse{}
}

// executeUpdatePrivate - Post an Internal Comment to a case and notify agents
func executeUpdatePrivate(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {

	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	commentLine := parseCommentLine("(\\/zendesk\\s*update\\s*private\\s*\\d*)(.*)", commandArgs.Command)

	isPublic := false
	in := zendesk.Ticket{
		Comment: &zendesk.TicketComment{
			Public: &isPublic,
			Body:   &commentLine,
		},
	}

	updatedTicket, err := p.zendeskClient.UpdateTicket(ticketNumber, &in)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	p.postCommandResponse(commandArgs, "Private comment ["+commentLine+"] was added to ticket #"+strconv.FormatInt(*updatedTicket.ID, 10))

	return &model.CommandResponse{}
}

// executeUpdatePublic - Post a Public Comment to a case and update all associated customer contacts and agents
func executeUpdatePublic(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {
	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	commentLine := parseCommentLine("(\\/zendesk\\s*update\\s*public\\s*\\d*)(.*)", commandArgs.Command)

	isPublic := true
	in := zendesk.Ticket{
		Comment: &zendesk.TicketComment{
			Public: &isPublic,
			Body:   &commentLine,
		},
	}

	updatedTicket, err := p.zendeskClient.UpdateTicket(ticketNumber, &in)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	p.postCommandResponse(commandArgs, "Public comment ["+commentLine+"] was added to ticket #"+strconv.FormatInt(*updatedTicket.ID, 10))

	return &model.CommandResponse{}
}

// executeLatestPrivate - Return the last internal comment posted to a case
func executeLatestPrivate(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 1 {
		return p.responsef(commandArgs, "Please specify a case number in the form `/zendesk latest private <case-number>`.")
	}

	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	ticketComments, err := p.zendeskClient.ListTicketComments(ticketNumber)
	if err != nil {
		return p.responsef(commandArgs, err.Error())
	}

	var lastPrivateComment zendesk.TicketComment
	for i := len(ticketComments) - 1; i >= 0; i-- {
		currentComment := ticketComments[i]
		if !*currentComment.Public {
			lastPrivateComment = currentComment
			break
		}
	}

	p.postCommandResponse(commandArgs, *lastPrivateComment.Body)

	return &model.CommandResponse{}
}

// executeLatestPublic -  Return the last Public Comment posted to a case
func executeLatestPublic(p *Plugin, c *plugin.Context, commandArgs *model.CommandArgs, args ...string) *model.CommandResponse {
	if len(args) != 1 {
		return p.responsef(commandArgs, "Please specify a case number in the form `/zendesk latest public <case-number>`.")
	}

	ticketNumber, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return p.responsef(commandArgs, err.Error())

	}

	ticketComments, err := p.zendeskClient.ListTicketComments(ticketNumber)
	if err != nil {
		return p.responsef(commandArgs, err.Error())
	}

	var lastPublicComment zendesk.TicketComment
	for i := len(ticketComments) - 1; i >= 0; i-- {
		currentComment := ticketComments[i]
		if *currentComment.Public {
			lastPublicComment = currentComment
			break
		}
	}

	p.postCommandResponse(commandArgs, *lastPublicComment.Body)

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

func parseCommentLine(regexString string, command string) string {
	re := regexp.MustCompile(regexString)
	commentLine := re.ReplaceAllString(command, "$2")

	return commentLine
}

func (p *Plugin) parseTicket(ticket *zendesk.Ticket, organization *zendesk.Organization) ([]*model.SlackAttachment, error) {
	ticketID := strconv.FormatInt(*ticket.ID, 10)

	text := fmt.Sprintf("[%s](%s%s)", ticketID+": "+*ticket.Subject, "https://my-testhelp.zendesk.com", "/agent/tickets/"+ticketID)
	desc := truncate(*ticket.Description, 3000)
	if desc != "" {
		text += "\n\n" + desc + "\n"
	}

	var fields []*model.SlackAttachmentField

	if ticket.Status != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Status",
			Value: *ticket.Status,
			Short: true,
		})
	}

	if ticket.AssigneeEmail != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Assignee",
			Value: *ticket.AssigneeEmail,
			Short: true,
		})
	}

	if ticket.Requester != nil && ticket.Requester.Name != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Requester",
			Value: *ticket.Requester.Name,
			Short: true,
		})
	}

	if organization != nil && organization.Name != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Organization",
			Value: *organization.Name,
			Short: true,
		})
	}

	if ticket.Priority != nil {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Priority",
			Value: *ticket.Priority,
			Short: true,
		})
	}

	return []*model.SlackAttachment{
		{
			Color:  "#95b7d0",
			Text:   text,
			Fields: fields,
		},
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max || max < 0 {
		return s
	}
	if max > 3 {
		return s[:max-3] + "..."
	}
	return s[:max]
}
