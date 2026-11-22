package api

import "context"

// Select chooses the attributes to return. Attributes is either a []string
// or the string "all"; use Attrs to build it.
type Select struct {
	Attributes any `json:"attributes"`
}

// Attrs converts a list of attribute names to the API's select form.
func Attrs(list []string) any {
	if len(list) == 1 && list[0] == "all" {
		return "all"
	}
	return list
}

// PersonInput identifies one person. Precedence on the server:
// person_id, linkedin_url, name + company_name/company_domain, email.
type PersonInput struct {
	PersonID      int64  `json:"person_id,omitempty"`
	LinkedInURL   string `json:"linkedin_url,omitempty"`
	Name          string `json:"name,omitempty"`
	CompanyName   string `json:"company_name,omitempty"`
	CompanyDomain string `json:"company_domain,omitempty"`
	JobTitle      string `json:"job_title,omitempty"`
}

// PeopleFilter is the filter-mode (search) input.
type PeopleFilter struct {
	Search               string   `json:"search,omitempty"`
	OrganizationIDs      []int64  `json:"organization_ids,omitempty"`
	Locations            []string `json:"locations,omitempty"`
	IncludePastEmployers bool     `json:"include_past_employers,omitempty"`
}

// PeopleRequest is the body of POST /v9/people. Set exactly one of People or Filter.
type PeopleRequest struct {
	People []PersonInput `json:"people,omitempty"`
	Filter *PeopleFilter `json:"filter,omitempty"`
	Select Select        `json:"select"`
	Limit  int           `json:"limit,omitempty"`
	Offset int           `json:"offset,omitempty"`
}

// People calls POST /v9/people and waits for the async result.
func (c *Client) People(ctx context.Context, req PeopleRequest) (*Response, error) {
	return c.Do(ctx, "/v9/people", req)
}

// OrgInput identifies one organization by id, slug, or name/url.
type OrgInput struct {
	ID   int64  `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// OrgsRequest is the match-mode body of POST /v9/organizations.
type OrgsRequest struct {
	Organizations []OrgInput `json:"organizations"`
	Select        Select     `json:"select"`
}

// Organizations calls POST /v9/organizations.
func (c *Client) Organizations(ctx context.Context, req OrgsRequest) (*Response, error) {
	return c.Do(ctx, "/v9/organizations", req)
}
