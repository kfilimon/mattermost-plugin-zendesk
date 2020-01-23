# Mattermost Zendesk Plugin

This plugin serves as a Prove Of Concept integration between Mattermost and Zendesk.

## What is currently covered
The following commands are implemented:
```
/zendesk status 12345 - Returns the current status of a case, I.e. Pending, Open, On-Hold, Solved Closed
/zendesk update private 12345 - Post an Internal Comment to a case and notify agents
/zendesk update public  12345 - Post a Public Comment to a case and update all associated customer contacts and agents
/zendesk latest private 12345 - Return the last internal comment posted to a case
/zendesk latest public 12345 - Return the last Public Comment posted to a case
/zendesk details 12345 - Return details of the case, Assignee, Requester, Organization, Issue, Priority, Status etc.
/zendesk connect - Connects the current Mattermost user with Zendesk (OAuth token is requested from Zendesk and stored in memory)
/zendesk disconnect - Disconnects the current Mattermost user from Zendesk (OAuth token is removed from the memory on Mattermost side)
/zendesk help - Shows a help message for the existing commands
```
![image](https://user-images.githubusercontent.com/17086299/73023882-b2f36480-3e2c-11ea-8388-3fb4b97fd094.png)

Three configuration properties will have to be modified after enabling the plugin: 

![image](https://user-images.githubusercontent.com/17086299/73024021-f9e15a00-3e2c-11ea-9889-9ae5caf78f45.png)

## Helpful resources
[Zendesk](https://www.zendesk.com/)

[Using OAuth authentication in Zendesk](https://support.zendesk.com/hc/en-us/articles/203663836-Using-OAuth-authentication-with-your-application) - step by step implementation of OAuth2 for Zendesk (in python)

[Mattermost Demo Plugin - Github](https://github.com/mattermost/mattermost-plugin-demo) - simple plugin for Mattermost

[Mattermost Jira Plugin - Github](https://github.com/mattermost/mattermost-plugin-jira) - Jira plugin - similar functionality as for Zendesk so could be used as an example

[Golang Client for Zendesk - Github](https://github.com/MEDIGO/go-zendesk) - go client for Zendesk (I forked it to add OAuth token support)


## Disclaimer
The current implementation is a POC and should be treated as such. 

Error handling, test coverage, code organization, OAuth implementation must be improved before considering this plugin ready for production use.

**Note**: only *status* command is using OAuth token for requesting data from Zendesk. Other commands rely on username/API token authentication (which should exist in corresponding environment variables on the Mattermost server)

