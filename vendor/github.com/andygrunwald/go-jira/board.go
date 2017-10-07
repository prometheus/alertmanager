package jira

import (
	"fmt"
	"time"
)

// BoardService handles Agile Boards for the JIRA instance / API.
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/server/
type BoardService struct {
	client *Client
}

// BoardsList reflects a list of agile boards
type BoardsList struct {
	MaxResults int     `json:"maxResults" structs:"maxResults"`
	StartAt    int     `json:"startAt" structs:"startAt"`
	Total      int     `json:"total" structs:"total"`
	IsLast     bool    `json:"isLast" structs:"isLast"`
	Values     []Board `json:"values" structs:"values"`
}

// Board represents a JIRA agile board
type Board struct {
	ID       int    `json:"id,omitempty" structs:"id,omitempty"`
	Self     string `json:"self,omitempty" structs:"self,omitempty"`
	Name     string `json:"name,omitempty" structs:"name,omitemtpy"`
	Type     string `json:"type,omitempty" structs:"type,omitempty"`
	FilterID int    `json:"filterId,omitempty" structs:"filterId,omitempty"`
}

// BoardListOptions specifies the optional parameters to the BoardService.GetList
type BoardListOptions struct {
	// BoardType filters results to boards of the specified type.
	// Valid values: scrum, kanban.
	BoardType string `url:"boardType,omitempty"`
	// Name filters results to boards that match or partially match the specified name.
	Name string `url:"name,omitempty"`
	// ProjectKeyOrID filters results to boards that are relevant to a project.
	// Relevance meaning that the JQL filter defined in board contains a reference to a project.
	ProjectKeyOrID string `url:"projectKeyOrId,omitempty"`

	SearchOptions
}

// Wrapper struct for search result
type sprintsResult struct {
	Sprints []Sprint `json:"values" structs:"values"`
}

// Sprint represents a sprint on JIRA agile board
type Sprint struct {
	ID            int        `json:"id" structs:"id"`
	Name          string     `json:"name" structs:"name"`
	CompleteDate  *time.Time `json:"completeDate" structs:"completeDate"`
	EndDate       *time.Time `json:"endDate" structs:"endDate"`
	StartDate     *time.Time `json:"startDate" structs:"startDate"`
	OriginBoardID int        `json:"originBoardId" structs:"originBoardId"`
	Self          string     `json:"self" structs:"self"`
	State         string     `json:"state" structs:"state"`
}

// GetAllBoards will returns all boards. This only includes boards that the user has permission to view.
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/cloud/#agile/1.0/board-getAllBoards
func (s *BoardService) GetAllBoards(opt *BoardListOptions) (*BoardsList, *Response, error) {
	apiEndpoint := "rest/agile/1.0/board"
	url, err := addOptions(apiEndpoint, opt)
	req, err := s.client.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}

	boards := new(BoardsList)
	resp, err := s.client.Do(req, boards)
	if err != nil {
		return nil, resp, err
	}

	return boards, resp, err
}

// GetBoard will returns the board for the given boardID.
// This board will only be returned if the user has permission to view it.
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/cloud/#agile/1.0/board-getBoard
func (s *BoardService) GetBoard(boardID int) (*Board, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/agile/1.0/board/%v", boardID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	board := new(Board)
	resp, err := s.client.Do(req, board)
	if err != nil {
		return nil, resp, err
	}
	return board, resp, nil
}

// CreateBoard creates a new board. Board name, type and filter Id is required.
// name - Must be less than 255 characters.
// type - Valid values: scrum, kanban
// filterId - Id of a filter that the user has permissions to view.
// Note, if the user does not have the 'Create shared objects' permission and tries to create a shared board, a private
// board will be created instead (remember that board sharing depends on the filter sharing).
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/cloud/#agile/1.0/board-createBoard
func (s *BoardService) CreateBoard(board *Board) (*Board, *Response, error) {
	apiEndpoint := "rest/agile/1.0/board"
	req, err := s.client.NewRequest("POST", apiEndpoint, board)
	if err != nil {
		return nil, nil, err
	}

	responseBoard := new(Board)
	resp, err := s.client.Do(req, responseBoard)
	if err != nil {
		return nil, resp, err
	}

	return responseBoard, resp, nil
}

// DeleteBoard will delete an agile board.
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/cloud/#agile/1.0/board-deleteBoard
func (s *BoardService) DeleteBoard(boardID int) (*Board, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/agile/1.0/board/%v", boardID)
	req, err := s.client.NewRequest("DELETE", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := s.client.Do(req, nil)
	return nil, resp, err
}

// GetAllSprints will returns all sprints from a board, for a given board Id.
// This only includes sprints that the user has permission to view.
//
// JIRA API docs: https://docs.atlassian.com/jira-software/REST/cloud/#agile/1.0/board/{boardId}/sprint
func (s *BoardService) GetAllSprints(boardID string) ([]Sprint, *Response, error) {
	apiEndpoint := fmt.Sprintf("rest/agile/1.0/board/%s/sprint", boardID)
	req, err := s.client.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, nil, err
	}

	result := new(sprintsResult)
	resp, err := s.client.Do(req, result)
	return result.Sprints, resp, err
}
