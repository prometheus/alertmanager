package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/google/go-querystring/query"
	"github.com/trivago/tgo/tcontainer"
)

const (
	// AssigneeAutomatic represents the value of the "Assignee: Automatic" of JIRA
	AssigneeAutomatic = "-1"
)

// IssueService handles Issues for the JIRA instance / API.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue
type IssueService struct {
	client *Client
}

// Issue represents a JIRA issue.
type Issue struct {
	Expand    string       `json:"expand,omitempty" structs:"expand,omitempty"`
	ID        string       `json:"id,omitempty" structs:"id,omitempty"`
	Self      string       `json:"self,omitempty" structs:"self,omitempty"`
	Key       string       `json:"key,omitempty" structs:"key,omitempty"`
	Fields    *IssueFields `json:"fields,omitempty" structs:"fields,omitempty"`
	Changelog *Changelog   `json:"changelog,omitempty" structs:"changelog,omitempty"`
}

// ChangelogItems reflects one single changelog item of a history item
type ChangelogItems struct {
	Field      string      `json:"field" structs:"field"`
	FieldType  string      `json:"fieldtype" structs:"fieldtype"`
	From       interface{} `json:"from" structs:"from"`
	FromString string      `json:"fromString" structs:"fromString"`
	To         interface{} `json:"to" structs:"to"`
	ToString   string      `json:"toString" structs:"toString"`
}

// ChangelogHistory reflects one single changelog history entry
type ChangelogHistory struct {
	Id      string           `json:"id" structs:"id"`
	Author  User             `json:"author" structs:"author"`
	Created string           `json:"created" structs:"created"`
	Items   []ChangelogItems `json:"items" structs:"items"`
}

// Changelog reflects the change log of an issue
type Changelog struct {
	Histories []ChangelogHistory `json:"histories,omitempty"`
}

// Attachment represents a JIRA attachment
type Attachment struct {
	Self      string `json:"self,omitempty" structs:"self,omitempty"`
	ID        string `json:"id,omitempty" structs:"id,omitempty"`
	Filename  string `json:"filename,omitempty" structs:"filename,omitempty"`
	Author    *User  `json:"author,omitempty" structs:"author,omitempty"`
	Created   string `json:"created,omitempty" structs:"created,omitempty"`
	Size      int    `json:"size,omitempty" structs:"size,omitempty"`
	MimeType  string `json:"mimeType,omitempty" structs:"mimeType,omitempty"`
	Content   string `json:"content,omitempty" structs:"content,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty" structs:"thumbnail,omitempty"`
}

// Epic represents the epic to which an issue is associated
// Not that this struct does not process the returned "color" value
type Epic struct {
	ID      int    `json:"id" structs:"id"`
	Key     string `json:"key" structs:"key"`
	Self    string `json:"self" structs:"self"`
	Name    string `json:"name" structs:"name"`
	Summary string `json:"summary" structs:"summary"`
	Done    bool   `json:"done" structs:"done"`
}

// IssueFields represents single fields of a JIRA issue.
// Every JIRA issue has several fields attached.
type IssueFields struct {
	// TODO Missing fields
	//      * "aggregatetimespent": null,
	//      * "workratio": -1,
	//      * "lastViewed": null,
	//      * "aggregatetimeoriginalestimate": null,
	//      * "aggregatetimeestimate": null,
	//      * "environment": null,
	Expand               string        `json:"expand,omitempty" structs:"expand,omitempty"`
	Type                 IssueType     `json:"issuetype" structs:"issuetype"`
	Project              Project       `json:"project,omitempty" structs:"project,omitempty"`
	Resolution           *Resolution   `json:"resolution,omitempty" structs:"resolution,omitempty"`
	Priority             *Priority     `json:"priority,omitempty" structs:"priority,omitempty"`
	Resolutiondate       string        `json:"resolutiondate,omitempty" structs:"resolutiondate,omitempty"`
	Created              string        `json:"created,omitempty" structs:"created,omitempty"`
	Duedate              string        `json:"duedate,omitempty" structs:"duedate,omitempty"`
	Watches              *Watches      `json:"watches,omitempty" structs:"watches,omitempty"`
	Assignee             *User         `json:"assignee,omitempty" structs:"assignee,omitempty"`
	Updated              string        `json:"updated,omitempty" structs:"updated,omitempty"`
	Description          string        `json:"description,omitempty" structs:"description,omitempty"`
	Summary              string        `json:"summary" structs:"summary"`
	Creator              *User         `json:"Creator,omitempty" structs:"Creator,omitempty"`
	Reporter             *User         `json:"reporter,omitempty" structs:"reporter,omitempty"`
	Components           []*Component  `json:"components,omitempty" structs:"components,omitempty"`
	Status               *Status       `json:"status,omitempty" structs:"status,omitempty"`
	Progress             *Progress     `json:"progress,omitempty" structs:"progress,omitempty"`
	AggregateProgress    *Progress     `json:"aggregateprogress,omitempty" structs:"aggregateprogress,omitempty"`
	TimeTracking         *TimeTracking `json:"timetracking,omitempty" structs:"timetracking,omitempty"`
	TimeSpent            int           `json:"timespent,omitempty" structs:"timespent,omitempty"`
	TimeEstimate         int           `json:"timeestimate,omitempty" structs:"timeestimate,omitempty"`
	TimeOriginalEstimate int           `json:"timeoriginalestimate,omitempty" structs:"timeoriginalestimate,omitempty"`
	Worklog              *Worklog      `json:"worklog,omitempty" structs:"worklog,omitempty"`
	IssueLinks           []*IssueLink  `json:"issuelinks,omitempty" structs:"issuelinks,omitempty"`
	Comments             *Comments     `json:"comment,omitempty" structs:"comment,omitempty"`
	FixVersions          []*FixVersion `json:"fixVersions,omitempty" structs:"fixVersions,omitempty"`
	Labels               []string      `json:"labels,omitempty" structs:"labels,omitempty"`
	Subtasks             []*Subtasks   `json:"subtasks,omitempty" structs:"subtasks,omitempty"`
	Attachments          []*Attachment `json:"attachment,omitempty" structs:"attachment,omitempty"`
	Epic                 *Epic         `json:"epic,omitempty" structs:"epic,omitempty"`
	Parent               *Parent       `json:"parent,omitempty" structs:"parent,omitempty"`
	Unknowns             tcontainer.MarshalMap
}

// MarshalJSON is a custom JSON marshal function for the IssueFields structs.
// It handles JIRA custom fields and maps those from / to "Unknowns" key.
func (i *IssueFields) MarshalJSON() ([]byte, error) {
	m := structs.Map(i)
	unknowns, okay := m["Unknowns"]
	if okay {
		// if unknowns present, shift all key value from unknown to a level up
		for key, value := range unknowns.(tcontainer.MarshalMap) {
			m[key] = value
		}
		delete(m, "Unknowns")
	}
	return json.Marshal(m)
}

// UnmarshalJSON is a custom JSON marshal function for the IssueFields structs.
// It handles JIRA custom fields and maps those from / to "Unknowns" key.
func (i *IssueFields) UnmarshalJSON(data []byte) error {

	// Do the normal unmarshalling first
	// Details for this way: http://choly.ca/post/go-json-marshalling/
	type Alias IssueFields
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(i),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	totalMap := tcontainer.NewMarshalMap()
	err := json.Unmarshal(data, &totalMap)
	if err != nil {
		return err
	}

	t := reflect.TypeOf(*i)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tagDetail := field.Tag.Get("json")
		if tagDetail == "" {
			// ignore if there are no tags
			continue
		}
		options := strings.Split(tagDetail, ",")

		if len(options) == 0 {
			return fmt.Errorf("No tags options found for %s", field.Name)
		}
		// the first one is the json tag
		key := options[0]
		if _, okay := totalMap.Value(key); okay {
			delete(totalMap, key)
		}

	}
	i = (*IssueFields)(aux.Alias)
	// all the tags found in the struct were removed. Whatever is left are unknowns to struct
	i.Unknowns = totalMap
	return nil

}

// IssueType represents a type of a JIRA issue.
// Typical types are "Request", "Bug", "Story", ...
type IssueType struct {
	Self        string `json:"self,omitempty" structs:"self,omitempty"`
	ID          string `json:"id,omitempty" structs:"id,omitempty"`
	Description string `json:"description,omitempty" structs:"description,omitempty"`
	IconURL     string `json:"iconUrl,omitempty" structs:"iconUrl,omitempty"`
	Name        string `json:"name,omitempty" structs:"name,omitempty"`
	Subtask     bool   `json:"subtask,omitempty" structs:"subtask,omitempty"`
	AvatarID    int    `json:"avatarId,omitempty" structs:"avatarId,omitempty"`
}

// Resolution represents a resolution of a JIRA issue.
// Typical types are "Fixed", "Suspended", "Won't Fix", ...
type Resolution struct {
	Self        string `json:"self" structs:"self"`
	ID          string `json:"id" structs:"id"`
	Description string `json:"description" structs:"description"`
	Name        string `json:"name" structs:"name"`
}

// Priority represents a priority of a JIRA issue.
// Typical types are "Normal", "Moderate", "Urgent", ...
type Priority struct {
	Self    string `json:"self,omitempty" structs:"self,omitempty"`
	IconURL string `json:"iconUrl,omitempty" structs:"iconUrl,omitempty"`
	Name    string `json:"name,omitempty" structs:"name,omitempty"`
	ID      string `json:"id,omitempty" structs:"id,omitempty"`
}

// Watches represents a type of how many user are "observing" a JIRA issue to track the status / updates.
type Watches struct {
	Self       string `json:"self,omitempty" structs:"self,omitempty"`
	WatchCount int    `json:"watchCount,omitempty" structs:"watchCount,omitempty"`
	IsWatching bool   `json:"isWatching,omitempty" structs:"isWatching,omitempty"`
}

// AvatarUrls represents different dimensions of avatars / images
type AvatarUrls struct {
	Four8X48  string `json:"48x48,omitempty" structs:"48x48,omitempty"`
	Two4X24   string `json:"24x24,omitempty" structs:"24x24,omitempty"`
	One6X16   string `json:"16x16,omitempty" structs:"16x16,omitempty"`
	Three2X32 string `json:"32x32,omitempty" structs:"32x32,omitempty"`
}

// Component represents a "component" of a JIRA issue.
// Components can be user defined in every JIRA instance.
type Component struct {
	Self string `json:"self,omitempty" structs:"self,omitempty"`
	ID   string `json:"id,omitempty" structs:"id,omitempty"`
	Name string `json:"name,omitempty" structs:"name,omitempty"`
}

// Status represents the current status of a JIRA issue.
// Typical status are "Open", "In Progress", "Closed", ...
// Status can be user defined in every JIRA instance.
type Status struct {
	Self           string         `json:"self" structs:"self"`
	Description    string         `json:"description" structs:"description"`
	IconURL        string         `json:"iconUrl" structs:"iconUrl"`
	Name           string         `json:"name" structs:"name"`
	ID             string         `json:"id" structs:"id"`
	StatusCategory StatusCategory `json:"statusCategory" structs:"statusCategory"`
}

// StatusCategory represents the category a status belongs to.
// Those categories can be user defined in every JIRA instance.
type StatusCategory struct {
	Self      string `json:"self" structs:"self"`
	ID        int    `json:"id" structs:"id"`
	Name      string `json:"name" structs:"name"`
	Key       string `json:"key" structs:"key"`
	ColorName string `json:"colorName" structs:"colorName"`
}

// Progress represents the progress of a JIRA issue.
type Progress struct {
	Progress int `json:"progress" structs:"progress"`
	Total    int `json:"total" structs:"total"`
}

// Parent represents the parent of a JIRA issue, to be used with subtask issue types.
type Parent struct {
	ID  string `json:"id,omitempty" structs:"id"`
	Key string `json:"key,omitempty" structs:"key"`
}

// Time represents the Time definition of JIRA as a time.Time of go
type Time time.Time

// Wrapper struct for search result
type transitionResult struct {
	Transitions []Transition `json:"transitions" structs:"transitions"`
}

// Transition represents an issue transition in JIRA
type Transition struct {
	ID     string                     `json:"id" structs:"id"`
	Name   string                     `json:"name" structs:"name"`
	Fields map[string]TransitionField `json:"fields" structs:"fields"`
}

// TransitionField represents the value of one Transition
type TransitionField struct {
	Required bool `json:"required" structs:"required"`
}

// CreateTransitionPayload is used for creating new issue transitions
type CreateTransitionPayload struct {
	Transition TransitionPayload `json:"transition" structs:"transition"`
}

// TransitionPayload represents the request payload of Transition calls like DoTransition
type TransitionPayload struct {
	ID string `json:"id" structs:"id"`
}

// Option represents an option value in a SelectList or MultiSelect
// custom issue field
type Option struct {
	Value string `json:"value" structs:"value"`
}

// UnmarshalJSON will transform the JIRA time into a time.Time
// during the transformation of the JIRA JSON response
func (t *Time) UnmarshalJSON(b []byte) error {
	ti, err := time.Parse("\"2006-01-02T15:04:05.999-0700\"", string(b))
	if err != nil {
		return err
	}
	*t = Time(ti)
	return nil
}

// Worklog represents the work log of a JIRA issue.
// One Worklog contains zero or n WorklogRecords
// JIRA Wiki: https://confluence.atlassian.com/jira/logging-work-on-an-issue-185729605.html
type Worklog struct {
	StartAt    int             `json:"startAt" structs:"startAt"`
	MaxResults int             `json:"maxResults" structs:"maxResults"`
	Total      int             `json:"total" structs:"total"`
	Worklogs   []WorklogRecord `json:"worklogs" structs:"worklogs"`
}

// WorklogRecord represents one entry of a Worklog
type WorklogRecord struct {
	Self             string `json:"self" structs:"self"`
	Author           User   `json:"author" structs:"author"`
	UpdateAuthor     User   `json:"updateAuthor" structs:"updateAuthor"`
	Comment          string `json:"comment" structs:"comment"`
	Created          Time   `json:"created" structs:"created"`
	Updated          Time   `json:"updated" structs:"updated"`
	Started          Time   `json:"started" structs:"started"`
	TimeSpent        string `json:"timeSpent" structs:"timeSpent"`
	TimeSpentSeconds int    `json:"timeSpentSeconds" structs:"timeSpentSeconds"`
	ID               string `json:"id" structs:"id"`
	IssueID          string `json:"issueId" structs:"issueId"`
}

// TimeTracking represents the timetracking fields of a JIRA issue.
type TimeTracking struct {
	OriginalEstimate         string `json:"originalEstimate,omitempty" structs:"originalEstimate,omitempty"`
	RemainingEstimate        string `json:"remainingEstimate,omitempty" structs:"remainingEstimate,omitempty"`
	TimeSpent                string `json:"timeSpent,omitempty" structs:"timeSpent,omitempty"`
	OriginalEstimateSeconds  int    `json:"originalEstimateSeconds,omitempty" structs:"originalEstimateSeconds,omitempty"`
	RemainingEstimateSeconds int    `json:"remainingEstimateSeconds,omitempty" structs:"remainingEstimateSeconds,omitempty"`
	TimeSpentSeconds         int    `json:"timeSpentSeconds,omitempty" structs:"timeSpentSeconds,omitempty"`
}

// Subtasks represents all issues of a parent issue.
type Subtasks struct {
	ID     string      `json:"id" structs:"id"`
	Key    string      `json:"key" structs:"key"`
	Self   string      `json:"self" structs:"self"`
	Fields IssueFields `json:"fields" structs:"fields"`
}

// IssueLink represents a link between two issues in JIRA.
type IssueLink struct {
	ID           string        `json:"id,omitempty" structs:"id,omitempty"`
	Self         string        `json:"self,omitempty" structs:"self,omitempty"`
	Type         IssueLinkType `json:"type" structs:"type"`
	OutwardIssue *Issue        `json:"outwardIssue" structs:"outwardIssue"`
	InwardIssue  *Issue        `json:"inwardIssue" structs:"inwardIssue"`
	Comment      *Comment      `json:"comment,omitempty" structs:"comment,omitempty"`
}

// IssueLinkType represents a type of a link between to issues in JIRA.
// Typical issue link types are "Related to", "Duplicate", "Is blocked by", etc.
type IssueLinkType struct {
	ID      string `json:"id,omitempty" structs:"id,omitempty"`
	Self    string `json:"self,omitempty" structs:"self,omitempty"`
	Name    string `json:"name" structs:"name"`
	Inward  string `json:"inward" structs:"inward"`
	Outward string `json:"outward" structs:"outward"`
}

// Comments represents a list of Comment.
type Comments struct {
	Comments []*Comment `json:"comments,omitempty" structs:"comments,omitempty"`
}

// Comment represents a comment by a person to an issue in JIRA.
type Comment struct {
	ID           string            `json:"id,omitempty" structs:"id,omitempty"`
	Self         string            `json:"self,omitempty" structs:"self,omitempty"`
	Name         string            `json:"name,omitempty" structs:"name,omitempty"`
	Author       User              `json:"author,omitempty" structs:"author,omitempty"`
	Body         string            `json:"body,omitempty" structs:"body,omitempty"`
	UpdateAuthor User              `json:"updateAuthor,omitempty" structs:"updateAuthor,omitempty"`
	Updated      string            `json:"updated,omitempty" structs:"updated,omitempty"`
	Created      string            `json:"created,omitempty" structs:"created,omitempty"`
	Visibility   CommentVisibility `json:"visibility,omitempty" structs:"visibility,omitempty"`
}

// FixVersion represents a software release in which an issue is fixed.
type FixVersion struct {
	Archived        *bool  `json:"archived,omitempty" structs:"archived,omitempty"`
	ID              string `json:"id,omitempty" structs:"id,omitempty"`
	Name            string `json:"name,omitempty" structs:"name,omitempty"`
	ProjectID       int    `json:"projectId,omitempty" structs:"projectId,omitempty"`
	ReleaseDate     string `json:"releaseDate,omitempty" structs:"releaseDate,omitempty"`
	Released        *bool  `json:"released,omitempty" structs:"released,omitempty"`
	Self            string `json:"self,omitempty" structs:"self,omitempty"`
	UserReleaseDate string `json:"userReleaseDate,omitempty" structs:"userReleaseDate,omitempty"`
}

// CommentVisibility represents he visibility of a comment.
// E.g. Type could be "role" and Value "Administrators"
type CommentVisibility struct {
	Type  string `json:"type,omitempty" structs:"type,omitempty"`
	Value string `json:"value,omitempty" structs:"value,omitempty"`
}

// SearchOptions specifies the optional parameters to various List methods that
// support pagination.
// Pagination is used for the JIRA REST APIs to conserve server resources and limit
// response size for resources that return potentially large collection of items.
// A request to a pages API will result in a values array wrapped in a JSON object with some paging metadata
// Default Pagination options
type SearchOptions struct {
	// StartAt: The starting index of the returned projects. Base index: 0.
	StartAt int `url:"startAt,omitempty"`
	// MaxResults: The maximum number of projects to return per page. Default: 50.
	MaxResults int `url:"maxResults,omitempty"`
	// Expand: Expand specific sections in the returned issues
	Expand string `url:"expand,omitempty"`
	Fields []string
}

// searchResult is only a small wrapper around the Search (with JQL) method
// to be able to parse the results
type searchResult struct {
	Issues     []Issue `json:"issues" structs:"issues"`
	StartAt    int     `json:"startAt" structs:"startAt"`
	MaxResults int     `json:"maxResults" structs:"maxResults"`
	Total      int     `json:"total" structs:"total"`
}

// GetQueryOptions specifies the optional parameters for the Get Issue methods
type GetQueryOptions struct {
	// Fields is the list of fields to return for the issue. By default, all fields are returned.
	Fields string `url:"fields,omitempty"`
	Expand string `url:"expand,omitempty"`
	// Properties is the list of properties to return for the issue. By default no properties are returned.
	Properties string `url:"properties,omitempty"`
	// FieldsByKeys if true then fields in issues will be referenced by keys instead of ids
	FieldsByKeys  bool `url:"fieldsByKeys,omitempty"`
	UpdateHistory bool `url:"updateHistory,omitempty"`
}

// CustomFields represents custom fields of JIRA
// This can heavily differ between JIRA instances
type CustomFields map[string]string

// Get returns a full representation of the issue for the given issue key.
// JIRA will attempt to identify the issue by the issueIdOrKey path parameter.
// This can be an issue id, or an issue key.
// If the issue cannot be found via an exact match, JIRA will also look for the issue in a case-insensitive way, or by looking to see if the issue was moved.
//
// The given options will be appended to the query string
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-getIssue
func (s *IssueService) Get(issueID string, options *GetQueryOptions) (*Issue, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s", issueID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	if options != nil {
		q, err := query.Values(options)
		if err != nil {
			return nil, nil, err
		}
		req.URL.RawQuery = q.Encode()
	}

	issue := new(Issue)
	resp, err := s.client.Do(req, issue)
	if err != nil {
		return nil, resp, err
	}

	return issue, resp, nil
}

// DownloadAttachment returns a Response of an attachment for a given attachmentID.
// The attachment is in the Response.Body of the response.
// This is an io.ReadCloser.
// The caller should close the resp.Body.
func (s *IssueService) DownloadAttachment(attachmentID string) (*Response, error) {
	apiEndpoint := fmt.Sprintf("secure/attachment/%s/", attachmentID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// PostAttachment uploads r (io.Reader) as an attachment to a given issueID
func (s *IssueService) PostAttachment(issueID string, r io.Reader, attachmentName string) (*[]Attachment, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s/attachments", issueID)

	b := new(bytes.Buffer)
	writer := multipart.NewWriter(b)

	fw, err := writer.CreateFormFile("file", attachmentName)
	if err != nil {
		return nil, nil, err
	}

	if r != nil {
		// Copy the file
		if _, err = io.Copy(fw, r); err != nil {
			return nil, nil, err
		}
	}
	writer.Close()

	req, err := s.client.NewMultiPartRequest("POST", apiEndpoint, b)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// PostAttachment response returns a JSON array (as multiple attachments can be posted)
	attachment := new([]Attachment)
	resp, err := s.client.Do(req, attachment)
	if err != nil {
		return nil, resp, err
	}

	return attachment, resp, nil
}

// Create creates an issue or a sub-task from a JSON representation.
// Creating a sub-task is similar to creating a regular issue, with two important differences:
// The issueType field must correspond to a sub-task issue type and you must provide a parent field in the issue create request containing the id or key of the parent issue.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-createIssues
func (s *IssueService) Create(issue *Issue) (*Issue, *Response, error) {
	apiEndpoint := "rest/api/2/issue/"
	req, err := s.client.NewRequest("POST", apiEndpoint, issue)
	if err != nil {
		return nil, nil, err
	}
	resp, err := s.client.Do(req, nil)
	if err != nil {
		// incase of error return the resp for further inspection
		return nil, resp, err
	}

	responseIssue := new(Issue)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, fmt.Errorf("Could not read the returned data")
	}
	err = json.Unmarshal(data, responseIssue)
	if err != nil {
		return nil, resp, fmt.Errorf("Could not unmarshall the data into struct")
	}
	return responseIssue, resp, nil
}

// Update updates an issue from a JSON representation. The issue is found by key.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/cloud/#api/2/issue-editIssue
func (s *IssueService) Update(issue *Issue) (*Issue, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%v", issue.Key)
	req, err := s.client.NewRequest("PUT", apiEndpoint, issue)
	if err != nil {
		return nil, nil, err
	}
	resp, err := s.client.Do(req, nil)
	if err != nil {
		return nil, resp, err
	}

	// This is just to follow the rest of the API's convention of returning an issue.
	// Returning the same pointer here is pointless, so we return a copy instead.
	ret := *issue
	return &ret, resp, nil
}

// Update updates an issue from a JSON representation. The issue is found by key.
//
// https://docs.atlassian.com/jira/REST/7.4.0/#api/2/issue-editIssue
func (s *IssueService) UpdateIssue(jiraId string, data map[string]interface{}) (*Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%v", jiraId)
	req, err := s.client.NewRequest("PUT", apiEndpoint, data)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	// This is just to follow the rest of the API's convention of returning an issue.
	// Returning the same pointer here is pointless, so we return a copy instead.
	return resp, nil
}

// AddComment adds a new comment to issueID.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-addComment
func (s *IssueService) AddComment(issueID string, comment *Comment) (*Comment, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s/comment", issueID)
	req, err := s.client.NewRequest("POST", apiEndpoint, comment)
	if err != nil {
		return nil, nil, err
	}

	responseComment := new(Comment)
	resp, err := s.client.Do(req, responseComment)
	if err != nil {
		return nil, resp, err
	}

	return responseComment, resp, nil
}

// UpdateComment updates the body of a comment, identified by comment.ID, on the issueID.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/cloud/#api/2/issue/{issueIdOrKey}/comment-updateComment
func (s *IssueService) UpdateComment(issueID string, comment *Comment) (*Comment, *Response, error) {
	reqBody := struct {
		Body string `json:"body"`
	}{
		Body: comment.Body,
	}
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s/comment/%s", issueID, comment.ID)
	req, err := s.client.NewRequest("POST", apiEndpoint, reqBody)
	if err != nil {
		return nil, nil, err
	}

	responseComment := new(Comment)
	resp, err := s.client.Do(req, responseComment)
	if err != nil {
		return nil, resp, err
	}

	return responseComment, resp, nil
}

// AddLink adds a link between two issues.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issueLink
func (s *IssueService) AddLink(issueLink *IssueLink) (*Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issueLink")
	req, err := s.client.NewRequest("POST", apiEndpoint, issueLink)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}

// Search will search for tickets according to the jql
//
// JIRA API docs: https://developer.atlassian.com/jiradev/jira-apis/jira-rest-apis/jira-rest-api-tutorials/jira-rest-api-example-query-issues
func (s *IssueService) Search(jql string, options *SearchOptions) ([]Issue, *Response, error) {
	var u string
	if options == nil {
		u = fmt.Sprintf("rest/api/2/search?jql=%s", url.QueryEscape(jql))
	} else {
		u = fmt.Sprintf("rest/api/2/search?jql=%s&startAt=%d&maxResults=%d&expand=%s&fields=%s", url.QueryEscape(jql),
			options.StartAt, options.MaxResults, options.Expand, strings.Join(options.Fields, ","))
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return []Issue{}, nil, err
	}

	v := new(searchResult)
	resp, err := s.client.Do(req, v)
	return v.Issues, resp, err
}

// GetCustomFields returns a map of customfield_* keys with string values
func (s *IssueService) GetCustomFields(issueID string) (CustomFields, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s", issueID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	issue := new(map[string]interface{})
	resp, err := s.client.Do(req, issue)
	if err != nil {
		return nil, resp, err
	}

	m := *issue
	f := m["fields"]
	cf := make(CustomFields)
	if f == nil {
		return cf, resp, nil
	}

	if rec, ok := f.(map[string]interface{}); ok {
		for key, val := range rec {
			if strings.Contains(key, "customfield") {
				if valMap, ok := val.(map[string]interface{}); ok {
					if v, ok := valMap["value"]; ok {
						val = v
					}
				}
				cf[key] = fmt.Sprint(val)
			}
		}
	}
	return cf, resp, nil
}

// GetTransitions gets a list of the transitions possible for this issue by the current user,
// along with fields that are required and their types.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-getTransitions
func (s *IssueService) GetTransitions(id string) ([]Transition, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s/transitions?expand=transitions.fields", id)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	result := new(transitionResult)
	resp, err := s.client.Do(req, result)
	return result.Transitions, resp, err
}

// DoTransition performs a transition on an issue.
// When performing the transition you can update or set other issue fields.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-doTransition
func (s *IssueService) DoTransition(ticketID, transitionID string) (*Response, error) {
	payload := CreateTransitionPayload{
		Transition: TransitionPayload{
			ID: transitionID,
		},
	}
	return s.DoTransitionWithPayload(ticketID, payload)
}

// DoTransitionWithPayload performs a transition on an issue using any payload.
// When performing the transition you can update or set other issue fields.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/issue-doTransition
func (s *IssueService) DoTransitionWithPayload(ticketID, payload interface{}) (*Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s/transitions", ticketID)

	req, err := s.client.NewRequest("POST", apiEndpoint, payload)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// InitIssueWithMetaAndFields returns Issue with with values from fieldsConfig properly set.
//  * metaProject should contain metaInformation about the project where the issue should be created.
//  * metaIssuetype is the MetaInformation about the Issuetype that needs to be created.
//  * fieldsConfig is a key->value pair where key represents the name of the field as seen in the UI
//		And value is the string value for that particular key.
// Note: This method doesn't verify that the fieldsConfig is complete with mandatory fields. The fieldsConfig is
//		 supposed to be already verified with MetaIssueType.CheckCompleteAndAvailable. It will however return
//		 error if the key is not found.
//		 All values will be packed into Unknowns. This is much convenient. If the struct fields needs to be
//		 configured as well, marshalling and unmarshalling will set the proper fields.
func InitIssueWithMetaAndFields(metaProject *MetaProject, metaIssuetype *MetaIssueType, fieldsConfig map[string]string) (*Issue, error) {
	issue := new(Issue)
	issueFields := new(IssueFields)
	issueFields.Unknowns = tcontainer.NewMarshalMap()

	// map the field names the User presented to jira's internal key
	allFields, _ := metaIssuetype.GetAllFields()
	for key, value := range fieldsConfig {
		jiraKey, found := allFields[key]
		if !found {
			return nil, fmt.Errorf("Key %s is not found in the list of fields.", key)
		}

		valueType, err := metaIssuetype.Fields.String(jiraKey + "/schema/type")
		if err != nil {
			return nil, err
		}
		switch valueType {
		case "array":
			elemType, err := metaIssuetype.Fields.String(jiraKey + "/schema/items")
			if err != nil {
				return nil, err
			}
			switch elemType {
			case "component":
				issueFields.Unknowns[jiraKey] = []Component{{Name: value}}
			default:
				issueFields.Unknowns[jiraKey] = []string{value}
			}
		case "string":
			issueFields.Unknowns[jiraKey] = value
		case "date":
			issueFields.Unknowns[jiraKey] = value
		case "datetime":
			issueFields.Unknowns[jiraKey] = value
		case "any":
			// Treat any as string
			issueFields.Unknowns[jiraKey] = value
		case "project":
			issueFields.Unknowns[jiraKey] = Project{
				Name: metaProject.Name,
				ID:   metaProject.Id,
			}
		case "priority":
			issueFields.Unknowns[jiraKey] = Priority{Name: value}
		case "user":
			issueFields.Unknowns[jiraKey] = User{
				Name: value,
			}
		case "issuetype":
			issueFields.Unknowns[jiraKey] = IssueType{
				Name: value,
			}
		case "option":
			issueFields.Unknowns[jiraKey] = Option{
				Value: value,
			}
		default:
			return nil, fmt.Errorf("Unknown issue type encountered: %s for %s", valueType, key)
		}
	}

	issue.Fields = issueFields

	return issue, nil
}

// Delete will delete a specified issue.
func (s *IssueService) Delete(issueID string) (*Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/issue/%s", issueID)

	// to enable deletion of subtasks; without this, the request will fail if the issue has subtasks
	deletePayload := make(map[string]interface{})
	deletePayload["deleteSubtasks"] = "true"
	content, _ := json.Marshal(deletePayload)

	req, err := s.client.NewRequest("DELETE", apiEndpoint, content)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}
