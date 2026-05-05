---
layout: page.njk
title: "Guides — velocity.report"
description: "How-to guides for building and running a velocity.report sensor: hardware, setup, and producing PDF reports from your data."
eleventyExcludeFromCollections: true
---

# How-To guides

Step-by-step tutorials to help you get the most out of velocity.report

<div class="card-grid guides">
{% for guide in collections.guides %}
{% if guide.url != page.url %}
<a href="{{ guide.url }}" class="block card hover:shadow-xl transition-shadow">
    <h3 class="card-title mb-2">{{ guide.data.title }}</h3>
    {% if guide.data.description %}
    <p class="card-description text-sm">{{ guide.data.description }}</p>
    {% endif %}
</a>
{% endif %}
{% endfor %}
</div>
