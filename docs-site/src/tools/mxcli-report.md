# mxcli report

The `mxcli report` command generates a scored best practices report for a Mendix project. It runs all lint rules and aggregates findings into a category-based scorecard.

## Basic Usage

```bash
# Generate a report (text output to terminal)
mxcli report -p app.mpr
```

## Output Formats

### Markdown

```bash
mxcli report -p app.mpr --format markdown
mxcli report -p app.mpr --format markdown --output report.md
```

### HTML

Visual report with color-coded scoring:

```bash
mxcli report -p app.mpr --format html --output report.html
```

### JSON

Machine-readable report for CI pipelines:

```bash
mxcli report -p app.mpr --format json
```

## Scoring Categories

The report scores the project across 6 categories on a 0-100 scale:

| Category | What It Measures |
|----------|-----------------|
| **Security** | Access rules, password policy, demo users, PII exposure |
| **Quality** | Documentation coverage, complexity, orphaned elements |
| **Architecture** | Module coupling, data access patterns, business keys |
| **Performance** | Commit-in-loop, query patterns |
| **Naming** | Entity, attribute, microflow, and page naming conventions |
| **Design** | Entity size, attribute counts, association patterns |

Each category shows:
- A score from 0 to 100
- Number of findings in that category
- Specific rule violations with affected elements

## Rules That Could Not Run

A lint rule that fails is a problem with the tooling, not the project, so it is
**not counted** in the score, the summary or any category. The report lists it
in its own section ("Rules That Could Not Run"; `ruleFailures` in JSON).

The common case is a rule written for a newer mxcli — one that reads a field
this binary's catalog does not expose. It is reported at info level as
`rule QUAL004 needs a newer mxcli ("microflow" struct has no .document_noun_title attribute)`.
Any other failure is reported as a `Starlark rule error`. If the project's
tooling was written by a newer mxcli, `mxcli` warns about it on every command;
see [Syncing with Updates](../ide/syncing.md#the-version-stamp).

## Writing Reports to Files

```bash
# Write HTML report
mxcli report -p app.mpr --format html --output report.html

# Write Markdown report
mxcli report -p app.mpr --format markdown --output report.md

# Write JSON report
mxcli report -p app.mpr --format json --output report.json
```

## CI Integration

Use the JSON format to fail CI pipelines when scores drop below a threshold:

```bash
# Check if overall score is above 70
SCORE=$(mxcli report -p app.mpr --format json | jq '.overallScore')
if [ "$SCORE" -lt 70 ]; then
  echo "Quality score $SCORE is below threshold 70"
  exit 1
fi
```
