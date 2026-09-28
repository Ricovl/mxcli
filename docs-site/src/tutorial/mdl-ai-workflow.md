# The MDL + AI Workflow

This page walks through the complete recommended workflow for using mxcli with an AI assistant, from project setup to reviewing the result in Studio Pro.

## The ten-step workflow

### Step 1: Initialize the project

Start by running `mxcli init` on your Mendix project:

```bash
mxcli init /path/to/my-mendix-project
```

This creates the configuration files, skill documents, and dev container setup. If you use a tool other than Claude Code, specify it with `--tool`:

```bash
mxcli init --tool cursor /path/to/my-mendix-project
```

### Step 2: Open in a dev container

Open the project folder in VS Code (or Cursor). VS Code will detect the `.devcontainer/` directory and prompt you to reopen in a container. Click **"Reopen in Container"**.

The dev container provides a sandboxed environment where the AI assistant can only access your project files. It comes with mxcli, JDK, Docker, and your AI tool pre-installed.

### Step 3: Start your AI assistant

Once inside the dev container, open a terminal and start your AI tool:

```bash
# Claude Code
claude

# Or use Cursor's Composer, Continue.dev's sidebar, etc.
```

### Step 4: Describe what you want

Tell the AI what you need in plain language. Be specific about module names and mention existing elements:

> "Create a Product entity in the Sales module with name, price, and description. Add an association to the existing Sales.Category entity."

Better prompts lead to better results. A few tips:

- Name the module explicitly: "in the Sales module"
- Reference existing elements: "linked to the existing Customer entity"
- Describe the behavior, not the implementation: "a microflow that validates the email format" rather than "an IF statement that checks for @ and ."

### Step 5: The AI explores your project

Before making changes, the AI uses mxcli to understand the current state of your project:

```sql
-- See what modules exist
LIST MODULES;

-- Check what's already in the Sales module
DESCRIBE STRUCTURE IN Sales;

-- Look at an existing entity for context
DESCRIBE ENTITY Sales.Category;

-- Search for related elements
SEARCH 'product';
```

This exploration step is important. The AI needs to know what entities, associations, and microflows already exist so it can write MDL that fits with your project.

### Step 6: The AI writes MDL

Guided by the skill files and its exploration, the AI writes an MDL script:

```sql
/** Product catalog item */
@Position(300, 100)
CREATE PERSISTENT ENTITY Sales.Product (
    Name: String(200) NOT NULL,
    Description: String(0),
    Price: Decimal NOT NULL DEFAULT 0,
    IsActive: Boolean DEFAULT true
);

/** Link products to categories */
CREATE ASSOCIATION Sales.Product_Category
    FROM Sales.Product
    TO Sales.Category
    TYPE Reference
    OWNER Default;
```

### Step 7: The AI validates

Before executing, the AI checks for errors:

```bash
# Check syntax
./mxcli check script.mdl

# Check syntax and verify that referenced elements exist
./mxcli check script.mdl -p app.mpr --references
```

If there are errors, the AI fixes them and re-validates. This cycle happens automatically -- you don't need to intervene.

### Step 8: The AI executes

Once validation passes, the AI runs the script against your project:

```bash
./mxcli -p app.mpr -c "EXECUTE SCRIPT 'script.mdl'"
```

The changes are written directly to your `.mpr` file.

### Step 9: Deep validation

For thorough validation, the AI can run the Mendix build tools:

```bash
./mxcli docker check -p app.mpr
```

This uses MxBuild (the same engine Studio Pro uses) to check the project for consistency errors, missing references, and other issues that `mxcli check` alone might miss.

### Step 10: Review in Studio Pro

Open your project in Mendix Studio Pro to review the changes visually. Check that:

- Entities appear correctly in the domain model
- Pages render as expected
- Microflow logic looks right in the visual editor
- Security settings are correct

## A complete example session

Here is what a full session looks like when you ask Claude Code to build a customer management feature:

**You:** "Create a customer management feature in the CRM module. I need a Customer entity with name, email, phone, and status. Create an overview page with a data grid and a popup edit form. Add a microflow that validates the email before saving."

**Claude Code:**

1. Runs `LIST MODULES` and `LIST ENTITIES IN CRM` to understand the project
2. Reads `generate-domain-model.md`, `overview-pages.md`, and `write-microflows.md` skills
3. Writes a script with the entity, enumeration, pages, and microflow
4. Validates with `mxcli check`
5. Executes the script
6. Runs `mxcli docker check` to verify
7. Reports back with a summary of what was created

The entire interaction takes a few minutes. The equivalent work in Studio Pro would take considerably longer.

## Tips for best results

### Be specific about what exists

If you're working with an existing project, mention the elements that are already there:

> "Add a ShippingAddress field to the existing CRM.Customer entity and update the CRM.Customer_Edit page to include it."

### Let the AI explore

Don't try to describe your entire project upfront. The AI is good at exploring. A prompt like "look at the Sales module and add a discount field to orders" is enough -- the AI will figure out the entity name, existing fields, and page structure on its own.

### Iterate

You don't have to get everything right in one prompt. Start with the entity, review it, then ask for pages, then microflows. Small, focused requests tend to produce better results than one massive prompt.

### Use `mxcli docker check` for final validation

`mxcli check` validates MDL syntax and references, but it doesn't catch everything. The Mendix build tools (`mxcli docker check`) perform the same validation that Studio Pro does. Use it as a final gate before considering the work done.

### Version control

Always work on a copy of your project or use version control. mxcli writes directly to your `.mpr` file. If something goes wrong, you want to be able to revert. For MPR v2 projects (Mendix 10.18+), the individual document files in `mprcontents/` work well with Git.

### Add custom skills for your project

If your project has specific patterns or conventions, write them down as skill files. For example, if every entity needs an audit trail (CreatedBy, CreatedAt, ChangedBy, ChangedAt), create a skill that documents this convention. The AI will follow it consistently.
