package jira

import (
	"fmt"
)

// GroupService handles Groups for the JIRA instance / API.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/server/#api/2/group
type GroupService struct {
	client *Client
}

// groupMembersResult is only a small wrapper around the Group* methods
// to be able to parse the results
type groupMembersResult struct {
	StartAt    int           `json:"startAt"`
	MaxResults int           `json:"maxResults"`
	Total      int           `json:"total"`
	Members    []GroupMember `json:"values"`
}

// GroupMember reflects a single member of a group
type GroupMember struct {
	Self         string `json:"self,omitempty"`
	Name         string `json:"name,omitempty"`
	Key          string `json:"key,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Active       bool   `json:"active,omitempty"`
	TimeZone     string `json:"timeZone,omitempty"`
}

// Get returns a paginated list of users who are members of the specified group and its subgroups.
// Users in the page are ordered by user names.
// User of this resource is required to have sysadmin or admin permissions.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/server/#api/2/group-getUsersFromGroup
func (s *GroupService) Get(name string) ([]GroupMember, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/group/member?groupname=%s", name)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	group := new(groupMembersResult)
	resp, err := s.client.Do(req, group)
	if err != nil {
		return nil, resp, err
	}

	return group.Members, resp, nil
}
