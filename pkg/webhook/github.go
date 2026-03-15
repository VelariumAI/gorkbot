package webhook

import (
	"encoding/json"
	"fmt"
)

// buildGitHubPrompt converts a GitHub webhook event + raw JSON payload into a
// natural-language prompt suitable for the AI agent.
func buildGitHubPrompt(event string, payload json.RawMessage) string {
	switch event {
	case "push":
		return buildPushPrompt(payload)
	case "pull_request":
		return buildPRPrompt(payload)
	case "issues":
		return buildIssuePrompt(payload)
	case "workflow_run":
		return buildWorkflowPrompt(payload)
	case "release":
		return buildReleasePrompt(payload)
	default:
		repo := extractRepo(payload)
		return fmt.Sprintf("GitHub event '%s' received in %s. Summarize what action is needed.", event, repo)
	}
}

func buildPushPrompt(payload json.RawMessage) string {
	var p struct {
		Ref        string `json:"ref"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Commits []struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.Repository.FullName == "" {
		return "A push event was received. Summarize the changes."
	}
	branch := p.Ref
	if len(branch) > 11 && branch[:11] == "refs/heads/" {
		branch = branch[11:]
	}
	msgs := ""
	for i, c := range p.Commits {
		if i >= 5 {
			msgs += fmt.Sprintf("  ... and %d more commits\n", len(p.Commits)-5)
			break
		}
		msgs += fmt.Sprintf("  - %s (%s)\n", c.Message, c.Author.Name)
	}
	if msgs == "" {
		msgs = "  (no commit details)"
	}
	return fmt.Sprintf("Summarize the changes in this push to %s on branch %s:\n%s",
		p.Repository.FullName, branch, msgs)
}

func buildPRPrompt(payload json.RawMessage) string {
	var p struct {
		Action      string `json:"action"`
		PullRequest struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			User  struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "A pull_request event was received. Review the changes."
	}
	body := p.PullRequest.Body
	if len(body) > 500 {
		body = body[:500] + "..."
	}
	return fmt.Sprintf("Review pull request '%s' (action: %s) in %s by %s:\n%s",
		p.PullRequest.Title, p.Action, p.Repository.FullName, p.PullRequest.User.Login, body)
}

func buildIssuePrompt(payload json.RawMessage) string {
	var p struct {
		Action string `json:"action"`
		Issue  struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			User  struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "A GitHub issue event was received. Analyze the issue."
	}
	body := p.Issue.Body
	if len(body) > 500 {
		body = body[:500] + "..."
	}
	return fmt.Sprintf("Analyze this issue (action: %s) in %s by %s: '%s' — %s",
		p.Action, p.Repository.FullName, p.Issue.User.Login, p.Issue.Title, body)
}

func buildWorkflowPrompt(payload json.RawMessage) string {
	var p struct {
		WorkflowRun struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
			Status     string `json:"status"`
		} `json:"workflow_run"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "A CI workflow event was received. Analyze the result."
	}
	return fmt.Sprintf("A CI workflow '%s' %s (status: %s) in %s. Analyze what happened and suggest fixes if needed.",
		p.WorkflowRun.Name, p.WorkflowRun.Conclusion, p.WorkflowRun.Status, p.Repository.FullName)
}

func buildReleasePrompt(payload json.RawMessage) string {
	var p struct {
		Release struct {
			TagName string `json:"tag_name"`
			Body    string `json:"body"`
		} `json:"release"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "A release event was received. Draft release notes."
	}
	body := p.Release.Body
	if len(body) > 500 {
		body = body[:500] + "..."
	}
	return fmt.Sprintf("Draft release notes for %s v%s:\n%s",
		p.Repository.FullName, p.Release.TagName, body)
}

func extractRepo(payload json.RawMessage) string {
	var p struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &p); err == nil {
		return p.Repository.FullName
	}
	return "unknown repository"
}
