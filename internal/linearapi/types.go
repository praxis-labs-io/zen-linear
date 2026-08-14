package linearapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shurcooL/graphql"
)

// parseTime safely parses an RFC3339 time string, returning zero time on error.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// IssueFilter is a custom scalar type for Linear's IssueFilter input.
// It allows passing complex filter objects to the GraphQL API.
type IssueFilter map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the filter.
func (IssueFilter) GetGraphQLType() string {
	return "IssueFilter"
}

// MarshalJSON implements json.Marshaler for IssueFilter.
func (f IssueFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(f))
}

// IssueCreateInput is a custom scalar type for Linear's IssueCreateInput.
// The Go type name must match the GraphQL type name exactly.
type IssueCreateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (IssueCreateInput) GetGraphQLType() string {
	return "IssueCreateInput"
}

// MarshalJSON implements json.Marshaler for IssueCreateInput.
func (i IssueCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// ProjectMilestoneFilter is a custom scalar type for Linear's ProjectMilestoneFilter input.
type ProjectMilestoneFilter map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the filter.
func (ProjectMilestoneFilter) GetGraphQLType() string {
	return "ProjectMilestoneFilter"
}

// MarshalJSON implements json.Marshaler for ProjectMilestoneFilter.
func (f ProjectMilestoneFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(f))
}

// IssueUpdateInput is a custom scalar type for Linear's IssueUpdateInput.
// The Go type name must match the GraphQL type name exactly.
type IssueUpdateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (IssueUpdateInput) GetGraphQLType() string {
	return "IssueUpdateInput"
}

// MarshalJSON implements json.Marshaler for IssueUpdateInput.
func (i IssueUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// CommentCreateInput is a custom scalar type for Linear's CommentCreateInput.
// The Go type name must match the GraphQL type name exactly.
type CommentCreateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (CommentCreateInput) GetGraphQLType() string {
	return "CommentCreateInput"
}

// MarshalJSON implements json.Marshaler for CommentCreateInput.
func (c CommentCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(c))
}

// CommentUpdateInput is a custom scalar type for Linear's CommentUpdateInput.
// The Go type name must match the GraphQL type name exactly.
type CommentUpdateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (CommentUpdateInput) GetGraphQLType() string {
	return "CommentUpdateInput"
}

// MarshalJSON implements json.Marshaler for CommentUpdateInput.
func (c CommentUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(c))
}

// IssueRelationCreateInput is a custom scalar type for Linear's IssueRelationCreateInput.
type IssueRelationCreateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (IssueRelationCreateInput) GetGraphQLType() string {
	return "IssueRelationCreateInput"
}

// MarshalJSON implements json.Marshaler for IssueRelationCreateInput.
func (i IssueRelationCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// FavoriteCreateInput is a custom scalar type for Linear's FavoriteCreateInput.
type FavoriteCreateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (FavoriteCreateInput) GetGraphQLType() string {
	return "FavoriteCreateInput"
}

// MarshalJSON implements json.Marshaler for FavoriteCreateInput.
func (i FavoriteCreateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// FavoriteUpdateInput is a custom scalar type for Linear's FavoriteUpdateInput.
type FavoriteUpdateInput map[string]interface{}

// GetGraphQLType returns the GraphQL type name for the input.
func (FavoriteUpdateInput) GetGraphQLType() string {
	return "FavoriteUpdateInput"
}

// MarshalJSON implements json.Marshaler for FavoriteUpdateInput.
func (i FavoriteUpdateInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}(i))
}

// FavoriteTarget names the entity a new favorite points at. Linear accepts
// exactly one target per favorite; the first field set here wins.
type FavoriteTarget struct {
	TeamID               string
	ProjectID            string
	CycleID              string
	CustomViewID         string
	IssueID              string
	PredefinedViewType   string
	PredefinedViewTeamID string
}

// input renders the target as Linear's create input, or nil when no entity is
// named.
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

// IssueRelationType is Linear's issue relation enum.
type IssueRelationType string

// GetGraphQLType returns the GraphQL type name for the enum.
func (IssueRelationType) GetGraphQLType() string {
	return "IssueRelationType"
}

const (
	IssueRelationBlocks    IssueRelationType = "blocks"
	IssueRelationRelated   IssueRelationType = "related"
	IssueRelationDuplicate IssueRelationType = "duplicate"
	IssueRelationSimilar   IssueRelationType = "similar"
)

// PaginationOrderBy is a custom type for Linear's PaginationOrderBy enum.
// Valid values are "createdAt" and "updatedAt".
type PaginationOrderBy string

// GetGraphQLType returns the GraphQL type name for the enum.
func (PaginationOrderBy) GetGraphQLType() string {
	return "PaginationOrderBy"
}

// Common PaginationOrderBy values.
const (
	OrderByCreatedAt PaginationOrderBy = "createdAt"
	OrderByUpdatedAt PaginationOrderBy = "updatedAt"
)

// Team represents a Linear team.
type Team struct {
	ID   string
	Key  string
	Name string
}

// Favorite represents an entry in the viewer's Linear favorites list. The
// nested object fields are populated according to Type; unsupported favorite
// types carry only ID, Type, and SortOrder.
type Favorite struct {
	ID        string
	Type      string // issue, project, cycle, team, customView, label, document, ...
	SortOrder float64

	IssueID         string
	IssueIdentifier string
	IssueTitle      string
	IssueTeamID     string

	ProjectID     string
	ProjectName   string
	ProjectTeamID string

	CycleID     string
	CycleName   string
	CycleNumber int
	CycleTeamID string

	TeamID   string
	TeamName string

	Title string // Linear's display label for the favorite

	// ParentID references the enclosing favorite folder, if any.
	ParentID string
	// FolderName is set for folder favorites.
	FolderName string

	CustomViewID   string
	CustomViewName string

	PredefinedViewType   string // e.g. "triage", "allIssues"
	PredefinedViewTeamID string
}

// Project represents a Linear project.
type Project struct {
	ID     string
	Name   string
	TeamID string
}

// ProjectMilestoneRef represents a lightweight reference to a Linear project milestone.
type ProjectMilestoneRef struct {
	ID         string
	Name       string
	ProjectID  string
	TargetDate *string
	Status     string
	SortOrder  float64
	Progress   float64
}

// ProjectMilestone represents a Linear project milestone.
type ProjectMilestone = ProjectMilestoneRef

// CycleRef represents a lightweight reference to a Linear cycle.
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

// DisplayName returns the user-facing cycle name, falling back to the cycle number.
func (c CycleRef) DisplayName() string {
	if strings.TrimSpace(c.Name) != "" {
		return c.Name
	}
	if c.Number > 0 {
		return fmt.Sprintf("Cycle %d", c.Number)
	}
	return "Cycle"
}

// Cycle represents a Linear cycle.
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

// DisplayName returns the user-facing cycle name, falling back to the cycle number.
func (c Cycle) DisplayName() string {
	return CycleRef{Name: c.Name, Number: c.Number}.DisplayName()
}

// User represents a Linear user.
type User struct {
	ID          string
	Name        string
	DisplayName string
	Email       string
	IsMe        bool
}

// WorkflowState represents a workflow state in a Linear team.
type WorkflowState struct {
	ID       string
	Name     string
	Type     string // backlog, unstarted, started, completed, canceled
	Position float64
	TeamID   string
}

// IssueLabel represents a label that can be applied to issues.
type IssueLabel struct {
	ID    string
	Name  string
	Color string // Hex color code (e.g., "#ff0000")
	// TeamID is set for a team-scoped label and empty for a workspace one.
	// Only the owning team may put a scoped label on an issue.
	TeamID string
}

// IssueRef represents a lightweight reference to an issue (for parent relationships).
type IssueRef struct {
	ID         string
	Identifier string
	Title      string
}

// IssueChildRef represents a lightweight reference to a child issue.
type IssueChildRef struct {
	ID         string
	Identifier string
	Title      string
	State      string
	StateID    string
}

// Comment represents a comment on a Linear issue.
type Comment struct {
	ID        string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    User
	IssueID   string
	// ParentID is the comment this one replies to, empty on a top-level
	// comment. Linear's threads are one level deep.
	ParentID string
	// URL is the comment's permalink, the anchor Linear opens the thread on.
	URL string
}

// IssueActivityKind names the change an activity event records. The details
// pane switches on it for the icon and the phrase.
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

// IssueActivity is one entry in an issue's activity feed: who changed what, and
// when. The fields below are populated according to Kind, the way a Favorite's
// are by Type; everything else stays zero.
//
// The entities named here are partial on purpose. The feed names a state, a
// cycle, a project; it does not open one, so Position, TeamID and the cycle's
// scheduling flags stay zero.
type IssueActivity struct {
	// ID is the history entry this came from, or the issue id on the derived
	// creation event. One entry can record several changes, so this is not
	// unique across the slice and nothing may key a map on it alone.
	ID   string
	Kind IssueActivityKind
	// CreatedAt is the merge key the feed sorts on. Never zero: an event with
	// no time would sort above the issue's own creation.
	CreatedAt time.Time
	// Actor is who made the change, zero when Linear records none. An
	// integration acting in place of a person arrives here under its own name.
	Actor User

	FromState *WorkflowState
	ToState   *WorkflowState
	// Assignee is who the issue went to on assigned and selfAssigned, and who
	// it came off on unassigned.
	Assignee  *User
	Cycle     *CycleRef
	Project   *Project
	Milestone *ProjectMilestoneRef
	Parent    *IssueRef
	Labels    []IssueLabel
	// RelatedIssue is the identifier a relation change named. It is a bare
	// identifier, not an id, so nothing in the feed can navigate to it, and it
	// may name an issue the reader cannot open.
	RelatedIssue string
	// Relation reads from the fetched issue's perspective, matching
	// IssueRelation.DisplayType: related, blocking, blocked by, duplicate,
	// duplicate of.
	Relation     string
	FromPriority int
	ToPriority   int
}

// IssueRelation represents a Linear issue relation.
type IssueRelation struct {
	ID           string
	Type         string
	Issue        IssueRef
	RelatedIssue IssueRef
	Inverse      bool
}

// DisplayType returns the relation label from the selected issue's perspective.
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

// Attachment represents an external resource linked to a Linear issue.
type Attachment struct {
	ID         string
	Title      string
	Subtitle   string
	URL        string
	SourceType string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Issue represents a Linear issue.
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
	Parent           *IssueRef       // Parent issue reference (nil if top-level)
	Children         []IssueChildRef // Child/sub-issue references
	Comments         []Comment       // Comments on this issue
	Activity         []IssueActivity // Oldest first; nil outside the detail fetch
	Relations        []IssueRelation
	Subscribers      []User
	Attachments      []Attachment
}

// IssueFetchProgress describes progress for a paginated issue fetch.
type IssueFetchProgress struct {
	Page    int
	Fetched int
}

// IssuePage represents a single page of issues with pagination info.
type IssuePage struct {
	Issues    []Issue
	HasNext   bool
	EndCursor *string
}

// FetchIssuesParams contains parameters for fetching issues.
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
	// CustomViewID fetches the issues of a Linear custom view instead of a
	// filtered query. Other filters are ignored when set.
	CustomViewID string
	// StateType filters by workflow state type (triage, backlog, unstarted,
	// started, completed, canceled).
	StateType string
	// IDs narrows the query to specific issues. Combined with the rest of the
	// params it answers whether an issue is inside the current scope, which no
	// local check can decide for a custom view.
	IDs []string
	// OrderBy specifies the sort order. Valid API values are "updatedAt" and "createdAt".
	// "priority" is also supported and will be sorted client-side after fetching.
	OrderBy string
	First   int
	// OnProgress is an optional callback invoked after each page is fetched.
	OnProgress func(IssueFetchProgress)
}

// DateFilter describes a Linear timeless date filter.
type DateFilter struct {
	Eq   string
	GT   string
	GTE  string
	LT   string
	LTE  string
	Null *bool
}

// Empty returns whether no date filter fields are set.
func (f DateFilter) Empty() bool {
	return f.Eq == "" && f.GT == "" && f.GTE == "" && f.LT == "" && f.LTE == "" && f.Null == nil
}

// NumberFilter describes a Linear numeric filter.
type NumberFilter struct {
	Eq   *float64
	GT   *float64
	GTE  *float64
	LT   *float64
	LTE  *float64
	Null *bool
}

// Empty returns whether no numeric filter fields are set.
func (f NumberFilter) Empty() bool {
	return f.Eq == nil && f.GT == nil && f.GTE == nil && f.LT == nil && f.LTE == nil && f.Null == nil
}

// CreateIssueInput contains input for creating a new issue.
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
	ParentID           string   // Parent issue ID (empty for top-level issues)
	LabelIDs           []string // empty means no labels; create has no "no change" state
	DueDate            string   // YYYY-MM-DD, empty for unset
	Estimate           *float64 // pointer because 0 is a legal estimate
}

// UpdateIssueInput contains input for updating an issue.
type UpdateIssueInput struct {
	ID                 string
	Title              *string
	Description        *string
	StateID            *string
	CycleID            *string // nil = no change, empty string = clear cycle, non-empty = set cycle
	AssigneeID         *string
	Priority           *int
	LabelIDs           *[]string // nil = no change, empty slice = clear all, non-empty = set labels
	ParentID           *string   // nil = no change, empty string = clear parent, non-empty = set parent
	DueDate            *string   // nil = no change, empty string = clear due date, non-empty = set YYYY-MM-DD date
	Estimate           *float64  // nil = no change, non-nil = set estimate
	ClearEstimate      bool      // true = clear estimate
	ProjectID          *string   // nil = no change, empty string = clear project, non-empty = set project
	ProjectMilestoneID *string   // nil = no change, empty string = clear milestone, non-empty = set milestone
	// TeamID moves the issue to another team. There is no clear: an issue
	// always belongs to a team.
	TeamID *string
}

// CreateCommentInput contains input for creating a new comment.
type CreateCommentInput struct {
	IssueID string
	Body    string
	// ParentID makes the comment a reply. Empty posts at top level.
	ParentID string
}

// UpdateCommentInput contains input for rewriting a comment's body.
type UpdateCommentInput struct {
	ID   string
	Body string
	// IssueID is the issue the comment sits on. The mutation finds the comment
	// by id alone; this is stamped onto the returned Comment, which the payload
	// is not asked for.
	IssueID string
}

// CreateIssueRelationInput contains input for creating an issue relation.
type CreateIssueRelationInput struct {
	IssueID        string
	RelatedIssueID string
	Type           IssueRelationType
}
