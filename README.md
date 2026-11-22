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
