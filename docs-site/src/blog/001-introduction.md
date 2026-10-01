# Introducing mxcli

*April 23, 2026*

---

Mendix is a powerful low-code platform, but its project format has always been a closed box. Your app lives in a `.mpr` file — a binary SQLite database with a BSON document layer on top. You can open it in Studio Pro and click around, but you cannot read it as text, diff it meaningfully in git, or script changes against it from the command line. For individual developers this is manageable. For teams doing large-scale migrations, automated generation, or AI-assisted development, it becomes a real bottleneck.

mxcli is our answer to that problem.

## What It Is

mxcli is a command-line tool that lets you read, query, and modify Mendix projects without Studio Pro. It exposes your `.mpr` file through **MDL** (Mendix Definition Language) — a SQL-like syntax that maps directly onto Mendix concepts:

```mdl
mdl 1;
-- Explore your project
LIST ENTITIES IN CustomerModule;
DESCRIBE ENTITY CustomerModule.Customer;

-- Make changes
CREATE PERSISTENT ENTITY CustomerModule.Order (
  TotalAmount: Decimal,
  Status: Enumeration(CustomerModule.OrderStatus)
);

CREATE ASSOCIATION CustomerModule.Order_Customer
  FROM CustomerModule.Order TO CustomerModule.Customer;
```

MDL scripts are plain text. They diff, they version-control, they compose. You can run them interactively in the REPL, execute them as `.mdl` files from CI, or drive them from an AI assistant.

## The AI Angle

The primary motivation for building mxcli was AI-assisted Mendix development. Tools like Claude Code and GitHub Copilot are transformative for backend and frontend work, but they cannot help with Mendix out of the box — the binary project format is opaque to them.

mxcli fixes this. With `mxcli init`, an AI assistant gets:
- A set of **skills** that explain the MDL language, common patterns, and validation steps
- A **CLAUDE.md** (or equivalent) that gives the assistant project-level context
- A language server and VS Code extension for `.mdl` files
- A sandboxed **Dev Container** for safe, reversible experimentation

The result is that an AI agent can explore your Mendix project, generate domain models, scaffold microflows and pages, apply security rules, and validate the result — all without you needing to describe the binary format or manually translate every instruction into clicks.

## Where We Are Today

mxcli is at **v0.7.0**. It covers the Mendix elements developers interact with most:

- Domain model — entities, attributes, associations, enumerations, indexes
- Microflows and nanoflows — 60+ activity types, control flow, expressions
- Pages and snippets — 50+ widget types, ALTER PAGE for in-place edits
- Security — module roles, user roles, entity access, demo users
- Navigation, workflows, business events, project settings
- External SQL — query PostgreSQL, Oracle, and SQL Server; import data
- Linting — 40+ built-in rules with SARIF output for CI integration
- Testing — MDL-based microflow tests with Docker integration

Forty-seven of Mendix's 52 metamodel domains are not yet covered (REST services, OData, and others are in progress), but for the day-to-day work of building a Mendix application, mxcli covers the essentials.

## What's Next

In the meantime, the best place to start is the [5-Minute Quickstart](../tutorial/quickstart.md). If you want to understand how AI-assisted development works with mxcli, the [MDL + AI Workflow](../tutorial/mdl-ai-workflow.md) guide walks through a complete example.

Questions, issues, and feature requests are welcome on [GitHub](https://github.com/mendixlabs/mxcli).
