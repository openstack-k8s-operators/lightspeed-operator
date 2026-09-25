# Configuration file for the Sphinx documentation builder.
# https://www.sphinx-doc.org/en/master/usage/configuration.html

project = "OpenStack Lightspeed Operator"
copyright = "OpenStack Lightspeed contributors"
author = "OpenStack Lightspeed contributors"

extensions = ["myst_parser", "sphinxcontrib.mermaid"]

# Docs are authored in Markdown (MyST).
# - colon_fence: write directives (toctree, mermaid) as ::: fenced blocks.
# - alert: render GitHub-style alerts (> [!NOTE], > [!WARNING], ...) as
#   admonitions, so the same source renders as callouts both here and on GitHub.
myst_enable_extensions = ["colon_fence", "alert"]

# Generate IDs for Markdown headings so standard relative links such as
# [Quota enforcement](configuration.md#quota-enforcement) work in the
# Sphinx output as well as on GitHub.  Level 3 covers every heading used as
# a target in these docs.
myst_heading_anchors = 3

# Treat ```mermaid fenced blocks as the {mermaid} directive, so the same plain
# fence renders as a diagram both on GitHub (native) and in the Sphinx build.
myst_fence_as_directive = ["mermaid"]

exclude_patterns = ["_build", ".venv", "Thumbs.db", ".DS_Store"]

html_theme = "sphinx_rtd_theme"
