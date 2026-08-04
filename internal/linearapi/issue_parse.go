package linearapi

import (
	"reflect"
)

func parseCycleRefValue(v reflect.Value) *CycleRef {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	id := reflectStringField(v, "ID")
	if id == "" {
		return nil
	}

	return &CycleRef{
		ID:         id,
		Name:       reflectStringField(v, "Name"),
		Number:     reflectIntField(v, "Number"),
		StartsAt:   parseTime(reflectStringField(v, "StartsAt")),
		EndsAt:     parseTime(reflectStringField(v, "EndsAt")),
		IsActive:   reflectBoolField(v, "IsActive"),
		IsFuture:   reflectBoolField(v, "IsFuture"),
		IsPast:     reflectBoolField(v, "IsPast"),
		IsNext:     reflectBoolField(v, "IsNext"),
		IsPrevious: reflectBoolField(v, "IsPrevious"),
	}
}

func parseProjectMilestoneRefValue(v reflect.Value) *ProjectMilestoneRef {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	id := reflectStringField(v, "ID")
	if id == "" {
		return nil
	}

	var targetDate *string
	if value := reflectStringField(v, "TargetDate"); value != "" {
		targetDate = &value
	}
	projectID := ""
	projectField := v.FieldByName("Project")
	if projectField.IsValid() {
		projectID = reflectStringField(projectField, "ID")
	}

	return &ProjectMilestoneRef{
		ID:         id,
		Name:       reflectStringField(v, "Name"),
		ProjectID:  projectID,
		TargetDate: targetDate,
		Status:     reflectStringField(v, "Status"),
		SortOrder:  reflectFloatField(v, "SortOrder"),
		Progress:   reflectFloatField(v, "Progress"),
	}
}

func reflectStringField(v reflect.Value, name string) string {
	if !v.IsValid() {
		return ""
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return ""
	}
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return ""
		}
		field = field.Elem()
	}
	if field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}

func reflectStringPointerField(v reflect.Value, name string) *string {
	value := reflectStringField(v, name)
	if value == "" {
		return nil
	}
	return &value
}

func reflectIntField(v reflect.Value, name string) int {
	if !v.IsValid() {
		return 0
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return 0
	}
	switch field.Kind() {
	case reflect.Float32, reflect.Float64:
		return int(field.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int())
	default:
		return 0
	}
}

func reflectFloatField(v reflect.Value, name string) float64 {
	if !v.IsValid() {
		return 0
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return 0
	}
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return 0
		}
		field = field.Elem()
	}
	switch field.Kind() {
	case reflect.Float32, reflect.Float64:
		return field.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(field.Int())
	default:
		return 0
	}
}

func reflectFloatPointerField(v reflect.Value, name string) *float64 {
	if !v.IsValid() {
		return nil
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return nil
	}
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			return nil
		}
		field = field.Elem()
	}
	switch field.Kind() {
	case reflect.Float32, reflect.Float64:
		value := field.Float()
		return &value
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := float64(field.Int())
		return &value
	default:
		return nil
	}
}

func reflectBoolField(v reflect.Value, name string) bool {
	if !v.IsValid() {
		return false
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		return false
	}
	if field.Kind() == reflect.Bool {
		return field.Bool()
	}
	return false
}

func parseIssueRefValue(v reflect.Value) IssueRef {
	return IssueRef{
		ID:         reflectStringField(v, "ID"),
		Identifier: reflectStringField(v, "Identifier"),
		Title:      reflectStringField(v, "Title"),
	}
}

func parseIssueRelationNodes(v reflect.Value, inverse bool) []IssueRelation {
	if !v.IsValid() {
		return nil
	}
	nodesField := v.FieldByName("Nodes")
	if !nodesField.IsValid() {
		return nil
	}
	relations := make([]IssueRelation, 0, nodesField.Len())
	for i := 0; i < nodesField.Len(); i++ {
		node := nodesField.Index(i)
		relations = append(relations, IssueRelation{
			ID:           reflectStringField(node, "ID"),
			Type:         reflectStringField(node, "Type"),
			Issue:        parseIssueRefValue(node.FieldByName("Issue")),
			RelatedIssue: parseIssueRefValue(node.FieldByName("RelatedIssue")),
			Inverse:      inverse,
		})
	}
	return relations
}

func parseUserNodes(v reflect.Value) []User {
	if !v.IsValid() {
		return nil
	}
	nodesField := v.FieldByName("Nodes")
	if !nodesField.IsValid() {
		return nil
	}
	users := make([]User, 0, nodesField.Len())
	for i := 0; i < nodesField.Len(); i++ {
		node := nodesField.Index(i)
		users = append(users, User{
			ID:          reflectStringField(node, "ID"),
			Name:        reflectStringField(node, "Name"),
			DisplayName: reflectStringField(node, "DisplayName"),
			Email:       reflectStringField(node, "Email"),
			IsMe:        reflectBoolField(node, "IsMe"),
		})
	}
	return users
}

func parseAttachmentNodes(v reflect.Value) []Attachment {
	if !v.IsValid() {
		return nil
	}
	nodesField := v.FieldByName("Nodes")
	if !nodesField.IsValid() {
		return nil
	}
	attachments := make([]Attachment, 0, nodesField.Len())
	for i := 0; i < nodesField.Len(); i++ {
		node := nodesField.Index(i)
		attachments = append(attachments, Attachment{
			ID:         reflectStringField(node, "ID"),
			Title:      reflectStringField(node, "Title"),
			Subtitle:   reflectStringField(node, "Subtitle"),
			URL:        reflectStringField(node, "URL"),
			SourceType: reflectStringField(node, "SourceType"),
			CreatedAt:  parseTime(reflectStringField(node, "CreatedAt")),
			UpdatedAt:  parseTime(reflectStringField(node, "UpdatedAt")),
		})
	}
	return attachments
}

// parseIssueNode converts a GraphQL issue node to an Issue struct.
func (c *Client) parseIssueNode(node interface{}) Issue {
	// Use type assertion to handle the node
	// This is a workaround since Go generics with GraphQL structs are complex
	v := reflect.ValueOf(node)

	id := v.FieldByName("ID").String()
	identifier := v.FieldByName("Identifier").String()
	title := v.FieldByName("Title").String()

	stateField := v.FieldByName("State")
	stateID := stateField.FieldByName("ID").String()
	stateName := stateField.FieldByName("Name").String()

	updatedAt := parseTime(v.FieldByName("UpdatedAt").String())
	createdAt := parseTime(v.FieldByName("CreatedAt").String())

	priority := int(v.FieldByName("Priority").Float())

	assignee := ""
	assigneeID := ""
	assigneeField := v.FieldByName("Assignee")
	if assigneeField.IsValid() && assigneeField.Kind() == reflect.Pointer && !assigneeField.IsNil() {
		assigneeID = assigneeField.Elem().FieldByName("ID").String()
		assignee = assigneeField.Elem().FieldByName("Name").String()
	}

	description := ""
	descField := v.FieldByName("Description")
	if descField.IsValid() && descField.Kind() == reflect.Pointer && !descField.IsNil() {
		description = descField.Elem().String()
	}

	teamID := v.FieldByName("Team").FieldByName("ID").String()

	projectID := ""
	projectName := ""
	projectField := v.FieldByName("Project")
	if projectField.IsValid() && projectField.Kind() == reflect.Pointer && !projectField.IsNil() {
		projectID = projectField.Elem().FieldByName("ID").String()
		projectName = reflectStringField(projectField.Elem(), "Name")
	}

	cycle := parseCycleRefValue(v.FieldByName("Cycle"))
	dueDate := reflectStringPointerField(v, "DueDate")
	estimate := reflectFloatPointerField(v, "Estimate")
	projectMilestone := parseProjectMilestoneRefValue(v.FieldByName("ProjectMilestone"))

	url := v.FieldByName("URL").String()
	branchName := reflectStringField(v, "BranchName")

	archivedField := v.FieldByName("ArchivedAt")
	archived := archivedField.IsValid() && archivedField.Kind() == reflect.Pointer && !archivedField.IsNil()

	// Parse labels
	labels := make([]IssueLabel, 0)
	labelsConn := v.FieldByName("Labels")
	if labelsConn.IsValid() {
		labelsField := labelsConn.FieldByName("Nodes")
		labels = make([]IssueLabel, 0, labelsField.Len())
		for i := 0; i < labelsField.Len(); i++ {
			lbl := labelsField.Index(i)
			labels = append(labels, IssueLabel{
				ID:    lbl.FieldByName("ID").String(),
				Name:  lbl.FieldByName("Name").String(),
				Color: lbl.FieldByName("Color").String(),
			})
		}
	}

	// Parse parent
	var parent *IssueRef
	parentField := v.FieldByName("Parent")
	if parentField.IsValid() && parentField.Kind() == reflect.Pointer && !parentField.IsNil() {
		parent = &IssueRef{
			ID:         parentField.Elem().FieldByName("ID").String(),
			Identifier: parentField.Elem().FieldByName("Identifier").String(),
			Title:      parentField.Elem().FieldByName("Title").String(),
		}
	}

	// Parse children
	children := make([]IssueChildRef, 0)
	childrenConn := v.FieldByName("Children")
	if childrenConn.IsValid() {
		childrenField := childrenConn.FieldByName("Nodes")
		children = make([]IssueChildRef, 0, childrenField.Len())
		for i := 0; i < childrenField.Len(); i++ {
			child := childrenField.Index(i)
			children = append(children, IssueChildRef{
				ID:         child.FieldByName("ID").String(),
				Identifier: child.FieldByName("Identifier").String(),
				Title:      child.FieldByName("Title").String(),
				State:      child.FieldByName("State").FieldByName("Name").String(),
				StateID:    child.FieldByName("State").FieldByName("ID").String(),
			})
		}
	}

	relations := make([]IssueRelation, 0)
	relations = append(relations, parseIssueRelationNodes(v.FieldByName("Relations"), false)...)
	relations = append(relations, parseIssueRelationNodes(v.FieldByName("InverseRelations"), true)...)
	subscribers := parseUserNodes(v.FieldByName("Subscribers"))
	attachments := parseAttachmentNodes(v.FieldByName("Attachments"))

	return Issue{
		ID:               id,
		Identifier:       identifier,
		Title:            title,
		State:            stateName,
		StateID:          stateID,
		Assignee:         assignee,
		AssigneeID:       assigneeID,
		Priority:         priority,
		UpdatedAt:        updatedAt,
		CreatedAt:        createdAt,
		Description:      description,
		TeamID:           teamID,
		ProjectID:        projectID,
		ProjectName:      projectName,
		Cycle:            cycle,
		DueDate:          dueDate,
		Estimate:         estimate,
		ProjectMilestone: projectMilestone,
		URL:              url,
		BranchName:       branchName,
		Archived:         archived,
		Labels:           labels,
		Parent:           parent,
		Children:         children,
		Relations:        relations,
		Subscribers:      subscribers,
		Attachments:      attachments,
	}
}
