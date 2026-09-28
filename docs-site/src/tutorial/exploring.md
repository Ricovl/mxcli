# Exploring a Project

Once you have a project open -- whether through the REPL, a CLI one-liner, or a script file -- the next step is to look around. What modules exist? What entities are defined? What does a particular microflow do?

mxcli provides three families of commands for exploration:

- **SHOW** commands list elements by type. `LIST ENTITIES` lists all entities; `LIST MICROFLOWS IN Sales` narrows the list to one module.
- **DESCRIBE** commands display the full MDL source for a single element, giving you the complete definition including attributes, associations, logic, and widget trees.
- **SEARCH** performs full-text search across every string in the project -- captions, messages, expressions, documentation, and more.
- **DESCRIBE STRUCTURE** gives you a compact tree view of the entire project or a single module, at varying levels of detail.

These commands are read-only. They never modify your project. You can run them freely to build a mental model of the application before making any changes.

## What you will learn

In this chapter, you will:

1. List modules, entities, microflows, pages, and other elements with **SHOW** commands
2. Inspect the full definition of any element with **DESCRIBE**
3. Find elements by keyword with **SEARCH**
4. Get a bird's-eye view of project structure with **DESCRIBE STRUCTURE**

## Prerequisites

You should have mxcli installed and know how to open a project. If not, work through the [Setting Up](setup.md) chapter first.

The examples in this chapter assume you have a Mendix project open. You can follow along using either the REPL or CLI one-liners:

```bash
# REPL (interactive)
mxcli -p /path/to/app.mpr

# CLI one-liner
mxcli -p /path/to/app.mpr -c "LIST ENTITIES"
```

If you do not have a Mendix project handy, the commands will still make sense -- the output format and options are the same regardless of the project content.
