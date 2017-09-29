package jira

import (
	"fmt"
)

// ProjectService handles projects for the JIRA instance / API.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/project
type ProjectService struct {
	client *Client
}

// ProjectList represent a list of Projects
type ProjectList []struct {
	Expand          string          `json:"expand" structs:"expand"`
	Self            string          `json:"self" structs:"self"`
	ID              string          `json:"id" structs:"id"`
	Key             string          `json:"key" structs:"key"`
	Name            string          `json:"name" structs:"name"`
	AvatarUrls      AvatarUrls      `json:"avatarUrls" structs:"avatarUrls"`
	ProjectTypeKey  string          `json:"projectTypeKey" structs:"projectTypeKey"`
	ProjectCategory ProjectCategory `json:"projectCategory,omitempty" structs:"projectsCategory,omitempty"`
}

// ProjectCategory represents a single project category
type ProjectCategory struct {
	Self        string `json:"self" structs:"self,omitempty"`
	ID          string `json:"id" structs:"id,omitempty"`
	Name        string `json:"name" structs:"name,omitempty"`
	Description string `json:"description" structs:"description,omitempty"`
}

// Project represents a JIRA Project.
type Project struct {
	Expand       string             `json:"expand,omitempty" structs:"expand,omitempty"`
	Self         string             `json:"self,omitempty" structs:"self,omitempty"`
	ID           string             `json:"id,omitempty" structs:"id,omitempty"`
	Key          string             `json:"key,omitempty" structs:"key,omitempty"`
	Description  string             `json:"description,omitempty" structs:"description,omitempty"`
	Lead         User               `json:"lead,omitempty" structs:"lead,omitempty"`
	Components   []ProjectComponent `json:"components,omitempty" structs:"components,omitempty"`
	IssueTypes   []IssueType        `json:"issueTypes,omitempty" structs:"issueTypes,omitempty"`
	URL          string             `json:"url,omitempty" structs:"url,omitempty"`
	Email        string             `json:"email,omitempty" structs:"email,omitempty"`
	AssigneeType string             `json:"assigneeType,omitempty" structs:"assigneeType,omitempty"`
	Versions     []Version          `json:"versions,omitempty" structs:"versions,omitempty"`
	Name         string             `json:"name,omitempty" structs:"name,omitempty"`
	Roles        struct {
		Developers string `json:"Developers,omitempty" structs:"Developers,omitempty"`
	} `json:"roles,omitempty" structs:"roles,omitempty"`
	AvatarUrls      AvatarUrls      `json:"avatarUrls,omitempty" structs:"avatarUrls,omitempty"`
	ProjectCategory ProjectCategory `json:"projectCategory,omitempty" structs:"projectCategory,omitempty"`
}

// Version represents a single release version of a project
type Version struct {
	Self            string `json:"self" structs:"self,omitempty"`
	ID              string `json:"id" structs:"id,omitempty"`
	Name            string `json:"name" structs:"name,omitempty"`
	Archived        bool   `json:"archived" structs:"archived,omitempty"`
	Released        bool   `json:"released" structs:"released,omitempty"`
	ReleaseDate     string `json:"releaseDate" structs:"releaseDate,omitempty"`
	UserReleaseDate string `json:"userReleaseDate" structs:"userReleaseDate,omitempty"`
	ProjectID       int    `json:"projectId" structs:"projectId,omitempty"` // Unlike other IDs, this is returned as a number
}

// ProjectComponent represents a single component of a project
type ProjectComponent struct {
	Self                string `json:"self" structs:"self,omitempty"`
	ID                  string `json:"id" structs:"id,omitempty"`
	Name                string `json:"name" structs:"name,omitempty"`
	Description         string `json:"description" structs:"description,omitempty"`
	Lead                User   `json:"lead,omitempty" structs:"lead,omitempty"`
	AssigneeType        string `json:"assigneeType" structs:"assigneeType,omitempty"`
	Assignee            User   `json:"assignee" structs:"assignee,omitempty"`
	RealAssigneeType    string `json:"realAssigneeType" structs:"realAssigneeType,omitempty"`
	RealAssignee        User   `json:"realAssignee" structs:"realAssignee,omitempty"`
	IsAssigneeTypeValid bool   `json:"isAssigneeTypeValid" structs:"isAssigneeTypeValid,omitempty"`
	Project             string `json:"project" structs:"project,omitempty"`
	ProjectID           int    `json:"projectId" structs:"projectId,omitempty"`
}

// GetList gets all projects form JIRA
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/project-getAllProjects
func (s *ProjectService) GetList() (*ProjectList, *Response, error) {
	apiEndpoint := "rest/api/2/project"
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	projectList := new(ProjectList)
	resp, err := s.client.Do(req, projectList)
	if err != nil {
		return nil, resp, err
	}
	return projectList, resp, nil
}

// Get returns a full representation of the project for the given issue key.
// JIRA will attempt to identify the project by the projectIdOrKey path parameter.
// This can be an project id, or an project key.
//
// JIRA API docs: https://docs.atlassian.com/jira/REST/latest/#api/2/project-getProject
func (s *ProjectService) Get(projectID string) (*Project, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/api/2/project/%s", projectID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	project := new(Project)
	resp, err := s.client.Do(req, project)
	if err != nil {
		return nil, resp, err
	}
	return project, resp, nil
}
