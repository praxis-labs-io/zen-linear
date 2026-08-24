package linearapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFavoritesTestClient points a client at a test server. The server ignores
// auth, so no credential is configured.
func newFavoritesTestClient(endpoint string) *Client {
	return NewClient(ClientConfig{Endpoint: endpoint})
}

// favoriteMutationServer captures the request body and replies with the given
// JSON data payload.
func favoriteMutationServer(t *testing.T, response string, captured *map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		var request map[string]interface{}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		*captured = request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
}

// marshalledInput renders a target the way it reaches Linear.
func marshalledInput(t *testing.T, target FavoriteTarget) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(target.input())
	if err != nil {
		t.Fatalf("marshaling input: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decoding input: %v", err)
	}
	return decoded
}

func TestFavoriteTargetInputPicksOneEntity(t *testing.T) {
	tests := []struct {
		name    string
		target  FavoriteTarget
		wantKey string
		wantVal string
	}{
		{"project", FavoriteTarget{ProjectID: "project-1"}, "projectId", "project-1"},
		{"team", FavoriteTarget{TeamID: "team-1"}, "teamId", "team-1"},
		{"cycle", FavoriteTarget{CycleID: "cycle-1"}, "cycleId", "cycle-1"},
		{"custom view", FavoriteTarget{CustomViewID: "view-1"}, "customViewId", "view-1"},
		{"issue", FavoriteTarget{IssueID: "issue-1"}, "issueId", "issue-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := marshalledInput(t, tt.target)
			if len(input) != 1 {
				t.Fatalf("input = %v, want a single key", input)
			}
			if input[tt.wantKey] != tt.wantVal {
				t.Errorf("input = %v, want %s %q", input, tt.wantKey, tt.wantVal)
			}
		})
	}
}

// TestFavoriteTargetInputPrefersCustomView guards the branch order: a custom
// view node also carries a team id, and the view has to win.
func TestFavoriteTargetInputPrefersCustomView(t *testing.T) {
	input := marshalledInput(t, FavoriteTarget{CustomViewID: "view-1", TeamID: "team-1"})

	if len(input) != 1 || input["customViewId"] != "view-1" {
		t.Fatalf("input = %v, want only customViewId view-1", input)
	}
}

func TestFavoriteTargetInputTriageCarriesTeam(t *testing.T) {
	input := marshalledInput(t, FavoriteTarget{
		PredefinedViewType:   "triage",
		PredefinedViewTeamID: "team-1",
	})

	if len(input) != 2 {
		t.Fatalf("input = %v, want predefinedViewType and predefinedViewTeamId", input)
	}
	if input["predefinedViewType"] != "triage" {
		t.Errorf("predefinedViewType = %v, want triage", input["predefinedViewType"])
	}
	if input["predefinedViewTeamId"] != "team-1" {
		t.Errorf("predefinedViewTeamId = %v, want team-1", input["predefinedViewTeamId"])
	}
}

func TestFavoriteTargetInputEmptyTarget(t *testing.T) {
	if input := (FavoriteTarget{}).input(); input != nil {
		t.Fatalf("input() = %v, want nil for an empty target", input)
	}
}

func TestCreateFavoriteParsesProject(t *testing.T) {
	response := `{"data":{"favoriteCreate":{"success":true,"favorite":{
		"id":"fav-1","type":"project","sortOrder":3.5,"title":"Website",
		"folderName":null,"parent":null,"predefinedViewType":null,
		"predefinedViewTeam":null,"customView":null,"issue":null,
		"project":{"id":"project-1","name":"Website"},
		"cycle":null,"team":null}}}}`

	var request map[string]interface{}
	server := favoriteMutationServer(t, response, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	favorite, err := client.CreateFavorite(context.Background(), FavoriteTarget{ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("CreateFavorite() error: %v", err)
	}
	if favorite.ID != "fav-1" || favorite.Type != "project" {
		t.Errorf("favorite = %+v, want fav-1 of type project", favorite)
	}
	if favorite.ProjectID != "project-1" || favorite.ProjectName != "Website" {
		t.Errorf("favorite project = %q/%q, want project-1/Website", favorite.ProjectID, favorite.ProjectName)
	}
	if favorite.SortOrder != 3.5 {
		t.Errorf("favorite SortOrder = %v, want 3.5", favorite.SortOrder)
	}

	variables, _ := request["variables"].(map[string]interface{})
	input, _ := variables["input"].(map[string]interface{})
	if input["projectId"] != "project-1" {
		t.Errorf("request input = %v, want projectId project-1", input)
	}
}

func TestCreateFavoriteRejectsEmptyTarget(t *testing.T) {
	client := newFavoritesTestClient("http://127.0.0.1:0")

	if _, err := client.CreateFavorite(context.Background(), FavoriteTarget{}); err == nil {
		t.Fatal("CreateFavorite() error = nil, want an error for an empty target")
	}
}

func TestCreateFavoriteReportsFailure(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteCreate":{"success":false,"favorite":{
		"id":"","type":"","sortOrder":0,"title":"","folderName":null,"parent":null,
		"predefinedViewType":null,"predefinedViewTeam":null,"customView":null,
		"issue":null,"project":null,"cycle":null,"team":null}}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if _, err := client.CreateFavorite(context.Background(), FavoriteTarget{TeamID: "team-1"}); err == nil {
		t.Fatal("CreateFavorite() error = nil, want an error when success is false")
	}
}

func TestDeleteFavorite(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteDelete":{"success":true}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.DeleteFavorite(context.Background(), "fav-1"); err != nil {
		t.Fatalf("DeleteFavorite() error: %v", err)
	}

	variables, _ := request["variables"].(map[string]interface{})
	if variables["id"] != "fav-1" {
		t.Errorf("request variables = %v, want id fav-1", variables)
	}
}

func TestDeleteFavoriteReportsFailure(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteDelete":{"success":false}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.DeleteFavorite(context.Background(), "fav-1"); err == nil {
		t.Fatal("DeleteFavorite() error = nil, want an error when success is false")
	}
}

func TestUpdateFavoriteSortOrder(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteUpdate":{"success":true}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.UpdateFavoriteSortOrder(context.Background(), "fav-1", 12.5); err != nil {
		t.Fatalf("UpdateFavoriteSortOrder() error: %v", err)
	}

	variables, _ := request["variables"].(map[string]interface{})
	if variables["id"] != "fav-1" {
		t.Errorf("request variables = %v, want id fav-1", variables)
	}
	input, _ := variables["input"].(map[string]interface{})
	if input["sortOrder"] != 12.5 {
		t.Errorf("request input = %v, want sortOrder 12.5", input)
	}
}

func TestUpdateFavoriteSortOrderReportsFailure(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteUpdate":{"success":false}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.UpdateFavoriteSortOrder(context.Background(), "fav-1", 1); err == nil {
		t.Fatal("UpdateFavoriteSortOrder() error = nil, want an error when success is false")
	}
}

func TestSortFavoritesOrdersBySortOrder(t *testing.T) {
	favorites := []Favorite{
		{ID: "c", SortOrder: 30},
		{ID: "a", SortOrder: 10},
		{ID: "b", SortOrder: 20},
	}

	SortFavorites(favorites)

	for i, want := range []string{"a", "b", "c"} {
		if favorites[i].ID != want {
			t.Fatalf("SortFavorites() = %+v, want a, b, c", favorites)
		}
	}
}

func TestMoveFavoriteIntoFolder(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteUpdate":{"success":true}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.MoveFavorite(context.Background(), "fav-1", "folder-1", 7); err != nil {
		t.Fatalf("MoveFavorite() error: %v", err)
	}

	variables, _ := request["variables"].(map[string]interface{})
	input, _ := variables["input"].(map[string]interface{})
	if input["parentId"] != "folder-1" {
		t.Errorf("request input = %v, want parentId folder-1", input)
	}
	if input["sortOrder"] != float64(7) {
		t.Errorf("request input = %v, want sortOrder 7", input)
	}
}

// TestMoveFavoriteToTopLevelSendsNullParent guards the detail that clears the
// folder: a blank string would be an invalid id, so the field has to be null.
func TestMoveFavoriteToTopLevelSendsNullParent(t *testing.T) {
	var request map[string]interface{}
	server := favoriteMutationServer(t, `{"data":{"favoriteUpdate":{"success":true}}}`, &request)
	defer server.Close()

	client := newFavoritesTestClient(server.URL)

	if err := client.MoveFavorite(context.Background(), "fav-1", "", 7); err != nil {
		t.Fatalf("MoveFavorite() error: %v", err)
	}

	variables, _ := request["variables"].(map[string]interface{})
	input, _ := variables["input"].(map[string]interface{})
	parent, present := input["parentId"]
	if !present {
		t.Fatalf("request input = %v, want an explicit parentId", input)
	}
	if parent != nil {
		t.Errorf("parentId = %v, want null", parent)
	}
}
