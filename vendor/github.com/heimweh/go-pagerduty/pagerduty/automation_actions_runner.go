package pagerduty

import "fmt"

// AutomationActionsRunner handles the communication with schedule
// related methods of the PagerDuty API.
type AutomationActionsRunnerService service

type AutomationActionsRunner struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	Type           string                       `json:"type"`
	RunnerType     string                       `json:"runner_type"`
	CreationTime   string                       `json:"creation_time"`
	LastSeenTime   *string                      `json:"last_seen,omitempty"`
	Summary        string                       `json:"summary,omitempty"`
	Description    *string                      `json:"description,omitempty"`
	RunbookBaseUri *string                      `json:"runbook_base_uri,omitempty"`
	RunbookApiKey  *string                      `json:"runbook_api_key,omitempty"`
	Teams          []*TeamReference             `json:"teams,omitempty"`
	Privileges     *AutomationActionsPrivileges `json:"privileges,omitempty"`
}

type AutomationActionsPrivileges struct {
	Permissions []*string `json:"permissions,omitempty"`
}

type AutomationActionsRunnerPayload struct {
	Runner *AutomationActionsRunner `json:"runner,omitempty"`
}

var automationActionsRunnerBaseUrl = "/automation_actions/runners"

// Create creates a new runner
func (s *AutomationActionsRunnerService) Create(runner *AutomationActionsRunner) (*AutomationActionsRunner, *Response, error) {
	u := automationActionsRunnerBaseUrl
	v := new(AutomationActionsRunnerPayload)

	resp, err := s.client.newRequestDoOptions("POST", u, nil, &AutomationActionsRunnerPayload{Runner: runner}, &v)
	if err != nil {
		return nil, nil, err
	}

	return v.Runner, resp, nil
}

// Get retrieves information about a runner.
func (s *AutomationActionsRunnerService) Get(id string) (*AutomationActionsRunner, *Response, error) {
	u := fmt.Sprintf("%s/%s", automationActionsRunnerBaseUrl, id)
	v := new(AutomationActionsRunnerPayload)

	resp, err := s.client.newRequestDoOptions("GET", u, nil, nil, &v)
	if err != nil {
		return nil, nil, err
	}

	return v.Runner, resp, nil
}

// Update an existing runner
func (s *AutomationActionsRunnerService) Update(ID string, runner *AutomationActionsRunner) (*AutomationActionsRunner, *Response, error) {
	u := fmt.Sprintf("%s/%s", automationActionsRunnerBaseUrl, ID)
	v := new(AutomationActionsRunnerPayload)
	p := &AutomationActionsRunnerPayload{Runner: runner}

	resp, err := s.client.newRequestDo("PUT", u, nil, p, v)
	if err != nil {
		return nil, nil, err
	}

	return v.Runner, resp, nil
}

// Delete deletes an existing runner.
func (s *AutomationActionsRunnerService) Delete(id string) (*Response, error) {
	u := fmt.Sprintf("%s/%s", automationActionsRunnerBaseUrl, id)

	return s.client.newRequestDoOptions("DELETE", u, nil, nil, nil)
}
