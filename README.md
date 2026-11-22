# sumble-cli

Unofficial, lightweight, dependency-free CLI for the [Sumble API](https://docs.sumble.com/api/api).

## Install

```sh
go install github.com/MaxHorstmann/sumble-cli/cmd/sumble@latest
```

## Auth

Set `SUMBLE_API_KEY` (create one at https://sumble.com/account/api-keys). The CLI exits
immediately if it is not set.

```sh
# For the current shell session (add to ~/.zshrc or ~/.bashrc to persist):
export SUMBLE_API_KEY=your-api-key
```

## Usage

```sh
sumble people find --name "Satya Nadella" --company-domain microsoft.com
sumble people find --linkedin https://www.linkedin.com/in/...
sumble people search "Salesforce admins" --org-id 10441 --location US:Texas --limit 25
sumble orgs find --url stripe.com
```

Common flags: `--attrs name,job_title,...` (or `all`), `--json`, `--timeout 2m`.

Output is a table by default; `--json` prints the raw API response. Credit usage is
printed to stderr. People requests are asynchronous on the API side; the CLI polls
until they finish.

Exit codes: `0` success, `1` error or no results, `2` usage error.

Credits: each matched record costs 1 credit plus 1 per paid attribute (`name`, and for
orgs `id`/`name`/`slug`/`url`/`sumble_url`, are free). The default people attributes
cost 5 credits per match.

`SUMBLE_API_BASE_URL` overrides the API host (used by tests).

## Example: weekly GitHub Actions report

[The example workflow](.github/workflows/weekly-sumble-report.yml) looks up my
LinkedIn profile every Monday at 09:00 UTC and posts the JSON response as a new
comment on [the tracking issue](https://github.com/MaxHorstmann/sumble-cli/issues/4).
It builds the CLI from this repository and runs:

```sh
sumble people find --linkedin "https://www.linkedin.com/in/max-horstmann" --attrs all --json
```

To enable it, add `SUMBLE_API_KEY` as a repository secret under **Settings >
Secrets and variables > Actions**. Once the workflow is on the default branch,
the weekly schedule is active; you can also run it from **Actions > What does
Sumble know about me? > Run workflow** (`workflow_dispatch`).

The workflow uses the built-in `GITHUB_TOKEN` with `issues: write` to post comments;
no separate GitHub token is needed. Each comment links to its workflow run.
Failed lookups (including no matches) also post a notice and leave the run failed.
`--attrs all` requests every available attribute and consumes Sumble credits for
each matched record and paid attribute.

**Results are public in this repository.** Only use a profile whose information
you intend to publish. To adapt the example, change the LinkedIn URL, hardcoded
issue number, and repository guard in the workflow. The guard prevents forks
from running this owner's report by default.
