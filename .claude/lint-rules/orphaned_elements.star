# Starlark Lint Rule: Orphaned Elements
#
# This rule checks for elements that are not referenced anywhere in the project.
# Orphaned elements may be:
#   - Dead code that should be removed
#   - Entry points that should be documented
#   - Work in progress that was forgotten
#
# Checks:
#   - Microflows, nanoflows and rules nothing references (except entry-point
#     prefixes such as ACT_)
#   - Pages nothing references (except likely entry pages)
#
# "Referenced" means what `mxcli refs` and CATALOG.GRAPH_DEAD_ASSETS mean: any
# edge in the catalog's reference graph that points at the document.
#
# This rule requires REFRESH CATALOG FULL to populate the refs table.
#
# Options (lint-config.yaml, rules.QUAL004.options):
#   entry_point_prefixes - microflow name prefixes that mark an entry point,
#                          as a YAML list or comma-separated text. Replaces
#                          the built-in list below; unset keeps it.
#
# Edit the option, not this file: an edited copy is kept, and no longer
# receives mxcli's updates to the rule.

RULE_ID = "QUAL004"
RULE_NAME = "Orphaned Elements"
DESCRIPTION = "Elements should be referenced somewhere in the project or marked as entry points"
CATEGORY = "quality"
SEVERITY = "info"

# Prefixes for microflows that are expected to be entry points, unless the
# entry_point_prefixes option says otherwise
ENTRY_POINT_PREFIXES = ["ACT_", "SCH_", "WS_", "REST_", "OData_"]

# Page name patterns that are likely entry points
ENTRY_PAGE_PATTERNS = ["Home", "Login", "Index", "Dashboard"]

# There is deliberately no list of the reference kinds that "count". Every edge
# into a microflow or a page is something that runs or opens it — a call, a
# widget, a calculated attribute, a scheduled event, a published service, an
# event handler, a project setting, the navigation — and the catalog keeps
# adding ways. A hand-kept list of them fell behind: it had the page kinds for a
# home page and a menu item but not the microflow ones, so a microflow started
# only from the navigation was reported with "Remove if unused" while
# `mxcli refs` listed the reference (ako/mxcli#1484). With no list, a new kind
# counts the day the catalog emits it.
#
# TestQUAL004CountsEveryRefKind (mdl/linter) drives every kind the catalog
# declares at a microflow, a nanoflow and a page, and fails on one that does not
# count.

def is_referenced(qualified_name):
    """True when anything in the catalog's reference graph points at the document."""
    return len(refs_to(qualified_name)) > 0

def is_entry_point_microflow(name, prefixes):
    """Check if a microflow name suggests it's a UI/scheduled entry point."""
    for prefix in prefixes:
        if name.startswith(prefix):
            return True
    return False

def is_entry_point_page(name):
    """Check if a page name suggests it's an entry point."""
    for pattern in ENTRY_PAGE_PATTERNS:
        if pattern in name:
            return True
    return False

def check():
    """
    Check for orphaned elements that have no incoming references.
    """
    violations = []
    entry_prefixes = get_list_option("entry_point_prefixes", ENTRY_POINT_PREFIXES)
    rename_hint = "/".join(entry_prefixes[:2]) if entry_prefixes else "an entry-point"

    # Check microflows
    for mf in microflows():
        # Skip entry point microflows
        if is_entry_point_microflow(mf.name, entry_prefixes):
            continue

        # Anything that causes the microflow to run counts as a caller, not just
        # a literal "call" edge. Each of these once reached a live microflow
        # that this rule then advised deleting: a page data source (DS_ microflows in
        # the banking-app report), the project's AfterStartupMicroflow (whose
        # removal mx check did not catch — the runtime refused to start), a
        # scheduled event, a published REST operation, an entity event handler,
        # and a home or menu-item microflow (ako/mxcli#1484).
        if not is_referenced(mf.qualified_name):
            loc = location(
                module=mf.module_name,
                document_type=mf.document_noun_title,
                document_name=mf.qualified_name
            )
            v = violation(
                message="{} '{}' is not called from anywhere.".format(mf.document_noun_title, mf.name),
                location=loc,
                suggestion="Remove if unused, rename with {} prefix if it's an entry point, or add its prefix to rules.QUAL004.options.entry_point_prefixes in lint-config.yaml.".format(rename_hint)
            )
            violations.append(v)

    # Check pages
    for page in pages():
        # Skip likely entry point pages
        if is_entry_point_page(page.name):
            continue

        # A page is reachable if anything opens it. Navigation counts: a page that
        # is only a home page, a login page or a menu item is reached by the
        # client, not by a microflow.
        if not is_referenced(page.qualified_name):
            loc = location(
                module=page.module_name,
                document_type="Page",
                document_name=page.qualified_name
            )
            v = violation(
                message="Page '{}' is not shown from any microflow.".format(page.name),
                location=loc,
                suggestion="Remove if unused, or verify it's configured as a menu item or home page."
            )
            violations.append(v)

    return violations
