package linearapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shurcooL/graphql"
)

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

type IssueFilter map[string]interface{}

func (IssueFilter) GetGraphQLType() string {
	return "IssueFilter"
}

func (f IssueFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(f))
}

type IssueCreateInput map[string]interface{}

func (IssueCreateInput) GetGraphQLType() string {
	return "IssueCreateInput"
}

func (i IssueCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

type ProjectMilestoneFilter map[string]interface{}

func (ProjectMilestoneFilter) GetGraphQLType() string {
	return "ProjectMilestoneFilter"
}

func (f ProjectMilestoneFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(f))
}

type IssueUpdateInput map[string]interface{}

func (IssueUpdateInput) GetGraphQLType() string {
	return "IssueUpdateInput"
}

func (i IssueUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

type CommentCreateInput map[string]interface{}

func (CommentCreateInput) GetGraphQLType() string {
	return "CommentCreateInput"
}

func (c CommentCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(c))
}

type CommentUpdateInput map[string]interface{}

func (CommentUpdateInput) GetGraphQLType() string {
	return "CommentUpdateInput"
}

func (c CommentUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(c))
}

type IssueRelationCreateInput map[string]interface{}

func (IssueRelationCreateInput) GetGraphQLType() string {
	return "IssueRelationCreateInput"
}

func (i IssueRelationCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

type FavoriteCreateInput map[string]interface{}

func (FavoriteCreateInput) GetGraphQLType() string {
	return "FavoriteCreateInput"
}

func (i FavoriteCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

type FavoriteUpdateInput map[string]interface{}

func (FavoriteUpdateInput) GetGraphQLType() string {
	return "FavoriteUpdateInput"
}

func (i FavoriteUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// FavoriteTarget names what a new favorite points at. The first field set wins.
type FavoriteTarget struct {
	TeamID               string
	ProjectID            string
	CycleID              string
	CustomViewID         string
	IssueID              string
	PredefinedViewType   string
	PredefinedViewTeamID string
}

func (t FavoriteTarget) input() FavoriteCreateInput {
	switch {
	case t.CustomViewID != "":
		return FavoriteCreateInput{"customViewId": graphql.String(t.CustomViewID)}
	case t.PredefinedViewType != "":
		input := FavoriteCreateInput{"predefinedViewType": graphql.String(t.PredefinedViewType)}
		if t.PredefinedViewTeamID != "" {
			input["predefinedViewTeamId"] = graphql.String(t.PredefinedViewTeamID)
		}
		return input
	case t.IssueID != "":
		return FavoriteCreateInput{"issueId": graphql.String(t.IssueID)}
	case t.CycleID != "":
		return FavoriteCreateInput{"cycleId": graphql.String(t.CycleID)}
	case t.ProjectID != "":
		return FavoriteCreateInput{"projectId": graphql.String(t.ProjectID)}
	case t.TeamID != "":
		return FavoriteCreateInput{"teamId": graphql.String(t.TeamID)}
	default:
		return nil
	}
}

type IssueRelationType string

func (IssueRelationType) GetGraphQLType() string {
	return "IssueRelationType"
}

const (
	IssueRelationBlocks    IssueRelationType = "blocks"
	IssueRelationRelated   IssueRelationType = "related"
	IssueRelationDuplicate IssueRelationType = "duplicate"
	IssueRelationSimilar   IssueRelationType = "similar"
)

type PaginationOrderBy string

func (PaginationOrderBy) GetGraphQLType() string {
	return "PaginationOrderBy"
}

const (
	OrderByCreatedAt PaginationOrderBy = "createdAt"
	OrderByUpdatedAt PaginationOrderBy = "updatedAt"
)

type Team struct {
	ID   string
	Key  string
	Name string
}

// Favorite is an entry in the viewer's favorites. Which nested fields are set
// depends on Type.
type Favorite struct {
	ID        string
	Type      string
	SortOrder float64

	IssueID         string
	IssueIdentifier string
	IssueTitle      string
	IssueTeamID     string

	ProjectID   string
	ProjectName string

	CycleID     string
	CycleName   string
	CycleNumber int
	CycleTeamID string

	TeamID   string
	TeamName string

	Title string

	ParentID   string
	FolderName string

	CustomViewID   string
	CustomViewName string

	PredefinedViewType   string
	PredefinedViewTeamID string
}

type Project struct {
	ID     string
	Name   string
	TeamID string
}

type ProjectMilestoneRef struct {
	ID         string
	Name       string
	ProjectID  string
	TargetDate *string
	Status     string
	SortOrder  float64
	Progress   float64
}

type ProjectMilestone = ProjectMilestoneRef

type CycleRef struct {
	ID         string
	Name       string
	Number     int
	StartsAt   time.Time
	EndsAt     time.Time
	IsActive   bool
	IsFuture   bool
	IsPast     bool
	IsNext     bool
	IsPrevious bool
}

func (c CycleRef) DisplayName() string {
	if strings.TrimSpace(c.Name) != "" {
		return c.Name
	}
	if c.Number > 0 {
		return fmt.Sprintf("Cycle %d", c.Number)
	}
	return "Cycle"
}

type Cycle struct {
	ID          string
	Name        string
	Number      int
	StartsAt    time.Time
	EndsAt      time.Time
	IsActive    bool
	IsFuture    bool
	IsPast      bool
	IsNext      bool
	IsPrevious  bool
	Description string
	TeamID      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c Cycle) DisplayName() string {
	return CycleRef{Name: c.Name, Number: c.Number}.DisplayName()
}

type User struct {
	ID          string
	Name        string
	DisplayName string
	Email       string
	IsMe        bool
}

type WorkflowState struct {
	ID       string
	Name     string
	Type     string
	Position float64
	TeamID   string
	// IsDefault marks the team's default state for a new issue. A team may have none.
	IsDefault bool
}

type IssueLabel struct {
	ID    string
	Name  string
	Color string
	// TeamID is empty for a workspace label. Only the owning team may apply a scoped one.
	TeamID string
}

type IssueRef struct {
	ID         string
	Identifier string
	Title      string
}

type IssueChildRef struct {
	ID         string
	Identifier string
	Title      string
	State      string
	StateID    string
}

type Comment struct {
	ID        string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    User
	IssueID   string
	// ParentID is the comment this one replies to, empty at top level.
	ParentID string
	URL      string
}

type IssueActivityKind string

const (
	IssueActivityCreated            IssueActivityKind = "created"
	IssueActivityStateChanged       IssueActivityKind = "stateChanged"
	IssueActivityAssigned           IssueActivityKind = "assigned"
	IssueActivitySelfAssigned       IssueActivityKind = "selfAssigned"
	IssueActivityUnassigned         IssueActivityKind = "unassigned"
	IssueActivityCycleAdded         IssueActivityKind = "cycleAdded"
	IssueActivityCycleRemoved       IssueActivityKind = "cycleRemoved"
	IssueActivityProjectSet         IssueActivityKind = "projectSet"
	IssueActivityProjectRemoved     IssueActivityKind = "projectRemoved"
	IssueActivityMilestoneSet       IssueActivityKind = "milestoneSet"
	IssueActivityMilestoneRemoved   IssueActivityKind = "milestoneRemoved"
	IssueActivityParentSet          IssueActivityKind = "parentSet"
	IssueActivityParentRemoved      IssueActivityKind = "parentRemoved"
	IssueActivityLabelsAdded        IssueActivityKind = "labelsAdded"
	IssueActivityLabelsRemoved      IssueActivityKind = "labelsRemoved"
	IssueActivityRelationAdded      IssueActivityKind = "relationAdded"
	IssueActivityRelationRemoved    IssueActivityKind = "relationRemoved"
	IssueActivityPriorityChanged    IssueActivityKind = "priorityChanged"
	IssueActivityTitleChanged       IssueActivityKind = "titleChanged"
	IssueActivityDescriptionUpdated IssueActivityKind = "descriptionUpdated"
)

// IssueActivity is one feed event. Which fields are set depends on Kind.
type IssueActivity struct {
	// ID is the history entry, shared by every event that entry produced.
	ID        string
	Kind      IssueActivityKind
	CreatedAt time.Time
	Actor     User

	FromState    *WorkflowState
	ToState      *WorkflowState
	Assignee     *User
	Cycle        *CycleRef
	Project      *Project
	Milestone    *ProjectMilestoneRef
	Parent       *IssueRef
	Labels       []IssueLabel
	RelatedIssue string
	Relation     string
	FromPriority int
	ToPriority   int
}

type IssueRelation struct {
	ID           string
	Type         string
	Issue        IssueRef
	RelatedIssue IssueRef
	Inverse      bool
}

func (r IssueRelation) DisplayType() string {
	switch r.Type {
	case string(IssueRelationBlocks):
		if r.Inverse {
			return "blocked by"
		}
		return "blocking"
	case string(IssueRelationDuplicate):
		if r.Inverse {
			return "duplicate of"
		}
		return "duplicate"
	case string(IssueRelationRelated):
		return "related"
	case string(IssueRelationSimilar):
		return "similar"
	default:
		if r.Inverse {
			return r.Type + " by"
		}
		return r.Type
	}
}

type Attachment struct {
	ID         string
	Title      string
	Subtitle   string
	URL        string
	SourceType string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Issue struct {
	ID               string
	Identifier       string
	Title            string
	Description      string
	State            string
	StateID          string
	Assignee         string
	AssigneeID       string
	Priority         int
	UpdatedAt        time.Time
	CreatedAt        time.Time
	TeamID           string
	ProjectID        string
	ProjectName      string
	BranchName       string
	Cycle            *CycleRef
	DueDate          *string
	Estimate         *float64
	ProjectMilestone *ProjectMilestoneRef
	URL              string
	Archived         bool
	Labels           []IssueLabel
	Parent           *IssueRef
	Children         []IssueChildRef
	Comments         []Comment
	Activity         []IssueActivity
	Relations        []IssueRelation
	Subscribers      []User
	Attachments      []Attachment
}

type IssueFetchProgress struct {
	Page    int
	Fetched int
}

type IssuePage struct {
	Issues    []Issue
	HasNext   bool
	EndCursor *string
}

type FetchIssuesParams struct {
	TeamID             string
	ProjectID          string
	StateID            string
	CycleID            string
	AssigneeID         string
	LabelIDs           []string
	ProjectMilestoneID string
	DueDate            DateFilter
	Estimate           NumberFilter
	Search             string
	// CustomViewID fetches a custom view's issues. Other filters are ignored when set.
	CustomViewID string
	StateType    string
	IDs          []string
	// OrderBy is "updatedAt", "createdAt", or "priority", which sorts client-side.
	OrderBy    string
	First      int
	OnProgress func(IssueFetchProgress)
}

type DateFilter struct {
	Eq   string
	GT   string
	GTE  string
	LT   string
	LTE  string
	Null *bool
}

func (f DateFilter) Empty() bool {
	return f.Eq == "" && f.GT == "" && f.GTE == "" && f.LT == "" && f.LTE == "" && f.Null == nil
}

type NumberFilter struct {
	Eq   *float64
	GT   *float64
	GTE  *float64
	LT   *float64
	LTE  *float64
	Null *bool
}

func (f NumberFilter) Empty() bool {
	return f.Eq == nil && f.GT == nil && f.GTE == nil && f.LT == nil && f.LTE == nil && f.Null == nil
}

type CreateIssueInput struct {
	TeamID             string
	Title              string
	Description        string
	ProjectID          string
	ProjectMilestoneID string
	StateID            string
	CycleID            string
	AssigneeID         string
	Priority           int
	ParentID           string
	LabelIDs           []string
	DueDate            string
	Estimate           *float64
}

type UpdateIssueInput struct {
	ID                 string
	Title              *string
	Description        *string
	StateID            *string
	CycleID            *string
	AssigneeID         *string
	Priority           *int
	LabelIDs           *[]string
	ParentID           *string
	DueDate            *string
	Estimate           *float64
	ClearEstimate      bool
	ProjectID          *string
	ProjectMilestoneID *string
	// TeamID moves the issue to another team. Nil or empty leaves it where it is.
	TeamID *string
}

type CreateCommentInput struct {
	IssueID string
	Body    string
	// ParentID makes the comment a reply. Empty posts at top level.
	ParentID string
}

type UpdateCommentInput struct {
	ID   string
	Body string
	// IssueID is stamped onto the returned Comment; the mutation finds it by id alone.
	IssueID string
}

type CreateIssueRelationInput struct {
	IssueID        string
	RelatedIssueID string
	Type           IssueRelationType
}
