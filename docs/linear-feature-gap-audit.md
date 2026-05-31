# Linear TUI Feature Gap Audit

This audit compares the current TUI surface with the main Linear API/product areas exposed by the app and the current Linear GraphQL schema. It is a follow-up backlog, not scope for the first cycle-support pass.

## Current Coverage

- Core issue list, search, sorting, details, comments, labels, parent/sub-issue relationships, archive, status, assignment, priority, projects, teams, and workflow states.
- First-pass cycle workflow: browse cycles under teams, filter issues by cycle, show cycle metadata on issues, set/clear an issue cycle, and select a cycle when creating issues.
- Local agent workflow over selected issues.

## Prioritized Gaps

### Issue Planning Fields

- Due dates: show, set, clear, and filter by due date.
- Estimates: show and edit issue estimate/points.
- Project milestones: list milestones, show milestone on issues, assign/clear milestone.
- Rich filters: combine assignee, labels, status, project, cycle, due date, estimate, and text search without relying only on navigation plus global search.

### Issue Relationships And Collaboration

- Issue relations/dependencies: blocked by, blocking, related, duplicate, and similar issue links.
- Subscribers: show subscribers and subscribe/unsubscribe the current user.
- Reactions: view and add emoji reactions on comments.
- Attachments and external links: show linked GitHub/Jira/Slack/URL attachments and open/copy them.

### P2: Project And Roadmap Workflow

- Project milestones and project updates.
- Roadmaps and initiatives.
- Project relations and project labels.
- Releases and release pipelines where teams use Linear for release planning.

### P2: Intake And Triage

- Triage inbox views and actions.
- Templates for issue creation.
- Custom views and saved filters.
- Delegated issues and agent/user delegation fields.

### P3: Workspace And Notification Surfaces

- Notifications, notification subscriptions, reminders, and snooze/archive workflows.
- Documents attached to teams, projects, initiatives, cycles, releases, and issues.
- Customers, customer needs, and customer tiers/statuses for product-intelligence workflows.
- Team settings and workflow-state management.

## Suggested Order

1. Add due date and estimate support because they are small issue-field extensions and improve planning immediately.
2. Add project milestone support because it reuses the cycle picker/navigation pattern.
3. Add dependency/relation support because it affects issue understanding and should be visible in details before editing.
4. Add richer filters or custom views once more issue metadata is available locally.
5. Add project updates, initiatives, releases, and notifications as separate feature slices.
