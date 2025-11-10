# velocity.report/docs

Documentation site for the velocity.report citizen radar system, built with Eleventy and Tailwind CSS.

**Location**: `docs/`
**Framework**: Eleventy (11ty)
**Styling**: Tailwind CSS
**Package Manager**: pnpm

## Prerequisites

- [Node.js](https://nodejs.org/) (v16 or higher)
- [pnpm](https://pnpm.io/) package manager

## Quick Start

```bash
# Install dependencies
pnpm install

# Start development server with hot reload
pnpm run dev
```

This runs Eleventy in watch mode with Tailwind CSS compilation. The site will be available at `http://localhost:8090`.

## Build

```bash
# Build for production
pnpm run build
```

Outputs optimized files to the `_site/` directory.

## Deployment

The site automatically deploys to GitHub Pages when changes are pushed to the `gh-pages` branch.

## Project Structure

```
docs/
├── _site/              # Build output (generated)
├── src/                # Source files
│   ├── _layouts/       # Nunjucks templates
│   ├── _includes/      # Reusable components
│   ├── guides/         # Markdown content
│   └── css/            # Styles with Tailwind
├── features/           # Feature specifications
├── database-migration-system-design.md  # Design docs
├── python-venv-consolidation-plan.md    # Design docs
├── package.json        # Dependencies and scripts
└── README.md           # This file
```

## Design Documents

The `docs/` directory also contains design documents and technical specifications:

- **[database-migration-system-design.md](./database-migration-system-design.md)** - Comprehensive design for database migration system with metadata tracking, rollback support, and tool evaluation (golang-migrate, goose, custom implementations)
- **[python-venv-consolidation-plan.md](./python-venv-consolidation-plan.md)** - Plan for consolidating Python virtual environments from dual-venv to single shared repository-level venv

These design documents follow the pattern:
- **Problem analysis** with current state and pain points
- **Design goals** and requirements
- **Solution options** with pros/cons comparison
- **Recommended approach** with detailed rationale
- **Implementation roadmap** with phased rollout
- **Risk assessment** and mitigation strategies
```

## Architecture: Markdown + Nunjucks Pattern

The docs site uses the **modern Eleventy pattern**: **Markdown for content, Nunjucks for layouts**.

This gives you:

- ✅ **Easy content authoring** with Markdown
- ✅ **Flexible styling** with Tailwind CSS + Nunjucks templates
- ✅ **Syntax highlighting** for code blocks
- ✅ **Typography optimized** for readability
- ✅ **Component reusability** with layouts and includes

### Content Flow

```
.md file (Markdown content)
    ↓
Front matter specifies layout
    ↓
Layout wraps content (Nunjucks)
    ↓
Base layout adds structure
    ↓
Final HTML with Tailwind styles
```

### File Types

1. **`.md` files** - Your content (guides, docs, pages)
2. **`.njk` files** - Templates and layouts (structure + styling)
3. **`.css` files** - Global styles with Tailwind

## Creating New Content

### 1. Markdown Guide (Recommended for most docs)

Create `src/guides/new-guide.md`:

```markdown
---
layout: doc.njk
title: Your Guide Title
description: Brief description for SEO and listings
section: guides
date: 2025-10-21
---

## Your Content Here

Write your content in Markdown. It will be automatically:

- Wrapped in the `doc.njk` layout
- Styled with Tailwind Typography
- Given syntax highlighting for code blocks

\`\`\`bash

# Code blocks work great

npm install something
\`\`\`

### Subsections

Use standard Markdown syntax.
```

### 2. Nunjucks Page (For custom layouts)

Create `src/custom-page.njk`:

```njk
---
layout: page.njk
title: Custom Page
---

<div class="max-w-7xl mx-auto">
    <h1 class="text-4xl font-bold">{{ title }}</h1>

    {# You have full Tailwind control here #}
    <div class="grid md:grid-cols-2 gap-6">
        <div class="bg-white p-6 rounded-lg shadow">
            Custom layout content
        </div>
    </div>
</div>
```

## Available Layouts

### `base.njk`

Minimal HTML structure. Use for landing pages with custom designs.

```markdown
---
layout: base.njk
---
```

### `page.njk`

Adds header and footer. Use for general pages.

```markdown
---
layout: page.njk
title: Page Title
---
```

### `doc.njk`

Full documentation layout with:

- Breadcrumb navigation
- Reading time estimate
- Prose styling (Tailwind Typography)
- Previous/Next navigation
- Help section with Discord/GitHub links

```markdown
---
layout: doc.njk
title: Documentation Page
description: What this page covers
section: guides
---
```

## Front Matter Variables

### Common to All Pages

```yaml
---
layout: doc.njk # Which layout to use
title: Page Title # Required - page title
description: Brief summary # Optional - for SEO and listings
date: 2025-10-21 # Optional - publication date
---
```

### Documentation Pages (doc.njk)

```yaml
---
section: guides # Creates breadcrumb (guides, reference, etc)
previousPage: # Manual prev/next navigation
  title: Previous Page
  url: /guides/prev/
nextPage:
  title: Next Page
  url: /guides/next/
---
```

## Tailwind Typography (Prose)

All Markdown content in `doc.njk` layout automatically gets beautiful typography:

```html
<article class="prose prose-lg max-w-none">
  <!-- Your Markdown content renders here -->
</article>
```

This styles:

- Headings with proper hierarchy
- Links in brand colors
- Code blocks with syntax highlighting
- Tables with borders
- Lists with proper spacing
- Blockquotes with left border

## Code Syntax Highlighting

Automatically enabled for all code blocks:

````markdown
```javascript
function hello() {
  console.log("Syntax highlighted!");
}
```

```bash
npm install package-name
```

```python
def calculate_speed(distance, time):
    return distance / time
```
````

## Collections

Collections group related content for navigation:

### Available Collections

- `collections.guides` - All files in `/src/guides/`
- `collections.gettingStarted` - All files in `/src/getting-started/`
- `collections.reference` - All files in `/src/reference/`

### Using Collections

In any `.njk` file:

```njk
{% for guide in collections.guides %}
  <a href="{{ guide.url }}">{{ guide.data.title }}</a>
{% endfor %}
```

## Custom Filters

### `readingTime`

Estimates reading time from content:

```njk
{{ content | readingTime }} min read
```

### `dateDisplay`

Formats dates nicely:

```njk
{{ date | dateDisplay }}
<!-- Output: October 21, 2025 -->
```

## Styling Components

### Using Tailwind Classes

In Markdown files, you can use HTML with Tailwind:

```markdown
## Standard Markdown Heading

<div class="bg-blue-50 border-l-4 border-blue-500 p-4 my-4">
  <p class="font-bold">Tip</p>
  <p>This is a custom styled callout using Tailwind classes!</p>
</div>

Back to regular Markdown...
```

### Custom Components

Define reusable components in `_includes/`:

Create `src/_includes/callout.njk`:

```njk
<div class="bg-{{ type }}-50 border-l-4 border-{{ type }}-500 p-4 my-4">
  <p class="font-bold">{{ title }}</p>
  {{ content | safe }}
</div>
```

Use in Markdown:

```markdown
{% include "callout.njk",
   type: "blue",
   title: "Note",
   content: "This is a reusable callout!" %}
```

## Development Workflow

### 1. Start Dev Server

```bash
cd docs
pnpm start
```

Visit: http://localhost:8090

### 2. Edit Content

- **Edit `.md` files** - Changes auto-reload
- **Edit `.njk` layouts** - Changes auto-reload
- **Edit `.css` files** - Tailwind rebuilds automatically

### 3. Build for Production

```bash
pnpm run build
```

Output in `_site/` directory.

## Best Practices

### ✅ DO

- Use Markdown (`.md`) for content-heavy pages (guides, docs, articles)
- Use Nunjucks (`.njk`) for structural pages (home, listings, custom layouts)
- Put reusable components in `_includes/`
- Use Tailwind utility classes for styling
- Use `prose` class for readable text content
- Create collections for related content groups

### ❌ DON'T

- Don't duplicate layouts - reuse existing ones
- Don't inline large styles - use Tailwind utilities
- Don't create `.njk` files when `.md` would work
- Don't forget front matter (layout, title, etc)

## Common Patterns

### Pattern 1: Simple Guide Page

```markdown
---
layout: doc.njk
title: Simple Guide
section: guides
---

Your Markdown content here...
```

### Pattern 2: Landing Page with Custom Layout

```njk
---
layout: page.njk
---

<div class="hero bg-blue-600 text-white py-20">
  <h1>Custom Hero Section</h1>
</div>

<div class="features grid grid-cols-3 gap-6">
  <!-- Custom layout code -->
</div>
```

### Pattern 3: Index/Listing Page

```markdown
---
layout: page.njk
title: All Guides
---

<div class="grid gap-6">
{% for guide in collections.guides %}
  <a href="{{ guide.url }}" class="feature-card">
    <h3>{{ guide.data.title }}</h3>
    <p>{{ guide.data.description }}</p>
  </a>
{% endfor %}
</div>
```

## Troubleshooting

### Layout not applying

Check front matter has `layout` key:

```yaml
---
layout: doc.njk # ← Must be present
title: My Page
---
```

### Styles not working

1. Ensure Tailwind is watching: `pnpm start`
2. Check `tailwind.config.js` includes your file type
3. Verify class names are correct

### Collection is empty

1. Files must be in correct directory (`src/guides/`)
2. Files must have `.md` extension
3. Check `.eleventy.js` collection definition

### Syntax highlighting not working

1. Ensure code blocks use triple backticks with language:

````markdown
```javascript
code here
```
````

2. Check `@11ty/eleventy-plugin-syntaxhighlight` is installed

## Next Steps

1. **Create more guides** - Use `first-report.md` as template
2. **Add images** - Create `src/images/` with screenshots
3. **Build homepage** - Enhance `index.njk` with hero + features
4. **Add reference docs** - Create `/reference/` directory
5. **Community section** - Add case studies and contribution guide

## Resources

- [Eleventy Documentation](https://www.11ty.dev/docs/)
- [Tailwind CSS](https://tailwindcss.com/docs)
- [Tailwind Typography](https://tailwindcss.com/docs/typography-plugin)
- [Markdown-it](https://markdown-it.github.io/)
- [Nunjucks Templating](https://mozilla.github.io/nunjucks/)
