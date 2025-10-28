---
layout: page.njk
title: Guides
eleventyExcludeFromCollections: true
---

# How-To Guides

Step-by-step tutorials to help you get the most out of velocity.report

<div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6 mt-8">
{% for guide in collections.guides %}
{% if guide.url != page.url %}
<a href="{{ guide.url }}" class="block p-6 bg-white rounded-lg shadow hover:shadow-lg transition-shadow border border-gray-200">
    <h3 class="text-xl font-semibold text-gray-900 mb-2">{{ guide.data.title }}</h3>
    {% if guide.data.description %}
    <p class="text-gray-600 text-sm">{{ guide.data.description }}</p>
    {% endif %}
</a>
{% endif %}
{% endfor %}
</div>
