package cli

import (
	"context"
	"strings"

	"github.com/MaxHorstmann/sumble-cli/internal/api"
)

// Defaults favour cheap attributes: each paid attribute costs 1 credit per
// matched record (name, id, slug, url, sumble_url are free).
const (
	defaultPeopleAttrs = "name,job_title,current_employer,location,linkedin_url"
	defaultOrgAttrs    = "id,name,slug,url,sumble_url"
)

func (a *app) peopleFind(ctx context.Context, args []string) int {
	fs := a.flagSet("sumble people find",
		"sumble people find --name NAME (--company NAME | --company-domain DOMAIN) [flags]\n"+
			"  sumble people find --linkedin URL [flags]\n"+
			"  sumble people find --person-id ID [flags]")
	var in api.PersonInput
	fs.StringVar(&in.Name, "name", "", "full name of the person")
	fs.StringVar(&in.CompanyName, "company", "", "employer name (with --name)")
	fs.StringVar(&in.CompanyDomain, "company-domain", "", "employer domain, e.g. microsoft.com (with --name)")
	fs.StringVar(&in.JobTitle, "title", "", "job title, helps rank candidates (with --name)")
	fs.StringVar(&in.LinkedInURL, "linkedin", "", "LinkedIn profile URL")
	fs.Int64Var(&in.PersonID, "person-id", 0, "Sumble person ID")
	var o outputOpts
	addCommon(fs, &o, defaultPeopleAttrs)

	pos, code, ok := a.parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		return a.usageErr(fs, "unexpected argument %q", pos[0])
	}
	switch {
	case in.PersonID != 0 || in.LinkedInURL != "":
	case in.Name != "" && (in.CompanyName != "" || in.CompanyDomain != ""):
	case in.Name != "":
		return a.usageErr(fs, "--name requires --company or --company-domain")
	default:
		return a.usageErr(fs, "specify --name with --company/--company-domain, --linkedin, or --person-id")
	}
	attrs, err := o.attributes()
	if err != nil {
		return a.usageErr(fs, "%v", err)
	}

	req := api.PeopleRequest{
		People: []api.PersonInput{in},
		Select: api.Select{Attributes: api.Attrs(attrs)},
	}
	return a.run(ctx, o, attrs, "people", func(ctx context.Context) (*api.Response, error) {
		return a.client.People(ctx, req)
	})
}

func (a *app) peopleSearch(ctx context.Context, args []string) int {
	fs := a.flagSet("sumble people search",
		"sumble people search QUERY [--org-id ID]... [--location CODE]... [flags]\n\n"+
			"Free-text search (requires a Pro or Enterprise plan).")
	var orgIDs int64List
	var locations stringList
	fs.Var(&orgIDs, "org-id", "restrict to organization ID (repeatable or comma-separated)")
	fs.Var(&locations, "location", "location code such as US, US:Texas, US:Texas:Austin (repeatable)")
	past := fs.Bool("past", false, "include people who previously worked at the organizations")
	limit := fs.Int("limit", 10, "maximum results (1-200)")
	offset := fs.Int("offset", 0, "results to skip (0-10000)")
	var o outputOpts
	addCommon(fs, &o, defaultPeopleAttrs)

	pos, code, ok := a.parse(fs, args)
	if !ok {
		return code
	}
	query := strings.TrimSpace(strings.Join(pos, " "))
	if query == "" {
		return a.usageErr(fs, "missing search query")
	}
	if *limit < 1 || *limit > 200 {
		return a.usageErr(fs, "--limit must be between 1 and 200")
	}
	if *offset < 0 || *offset > 10000 {
		return a.usageErr(fs, "--offset must be between 0 and 10000")
	}
	attrs, err := o.attributes()
	if err != nil {
		return a.usageErr(fs, "%v", err)
	}

	req := api.PeopleRequest{
		Filter: &api.PeopleFilter{
			Search:               query,
			OrganizationIDs:      orgIDs,
			Locations:            locations,
			IncludePastEmployers: *past,
		},
		Select: api.Select{Attributes: api.Attrs(attrs)},
		Limit:  *limit,
		Offset: *offset,
	}
	return a.run(ctx, o, attrs, "people", func(ctx context.Context) (*api.Response, error) {
		return a.client.People(ctx, req)
	})
}

func (a *app) orgsFind(ctx context.Context, args []string) int {
	fs := a.flagSet("sumble orgs find",
		"sumble orgs find --url DOMAIN | --slug SLUG | --id ID | --name NAME [flags]")
	var in api.OrgInput
	fs.StringVar(&in.URL, "url", "", "organization domain or website, e.g. stripe.com")
	fs.StringVar(&in.Slug, "slug", "", "Sumble slug (as in sumble.com/orgs/<slug>)")
	fs.Int64Var(&in.ID, "id", 0, "Sumble organization ID")
	fs.StringVar(&in.Name, "name", "", "organization name")
	var o outputOpts
	addCommon(fs, &o, defaultOrgAttrs)

	pos, code, ok := a.parse(fs, args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		return a.usageErr(fs, "unexpected argument %q", pos[0])
	}
	if in == (api.OrgInput{}) {
		return a.usageErr(fs, "specify --url, --slug, --id, or --name")
	}
	attrs, err := o.attributes()
	if err != nil {
		return a.usageErr(fs, "%v", err)
	}

	req := api.OrgsRequest{
		Organizations: []api.OrgInput{in},
		Select:        api.Select{Attributes: api.Attrs(attrs)},
	}
	return a.run(ctx, o, attrs, "organizations", func(ctx context.Context) (*api.Response, error) {
		return a.client.Organizations(ctx, req)
	})
}
