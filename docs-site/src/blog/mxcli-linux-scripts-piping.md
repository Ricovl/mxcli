# mxcli in Shell Scripts: Piping, JSON Output, and Automation

*Draft — not yet published*

---

<!-- TODO: date -->

<!-- TODO: intro — mxcli as a Unix-friendly CLI tool, composable with standard tooling -->

## Running Commands Non-Interactively

```bash
mxcli -p app.mpr -c "LIST ENTITIES"
mxcli -p app.mpr -c "DESCRIBE CustomerModule.Customer"
```

<!-- TODO: show one-liner patterns -->

## JSON Output

```bash
mxcli -p app.mpr -c "LIST ENTITIES" --json
mxcli -p app.mpr -c "LIST ENTITIES" --json | jq '.[] | .name'
```

<!-- TODO: cover --json flag across commands, structure of the output -->

## Executing Script Files

```bash
mxcli exec setup.mdl -p app.mpr
```

<!-- TODO: when to use script files vs one-liners -->

## Demo

<!-- TODO: embed YouTube demo — terminal session showing piped commands -->
<!--
<div style="position:relative;padding-bottom:56.25%;height:0;overflow:hidden;">
  <iframe style="position:absolute;top:0;left:0;width:100%;height:100%;"
    src="https://www.youtube.com/embed/VIDEO_ID"
    frameborder="0" allowfullscreen></iframe>
</div>
-->

## CI Pipeline Example

<!-- TODO: GitHub Actions or shell script snippet — lint + validate on every commit -->

## Other Export Formats

<!-- TODO: mxcli report --format markdown/html/json, mxcli lint --format sarif -->
