---
layout: page.njk
title: Guides
---

<div class="max-w-7xl mx-auto">
    <div class="text-center mb-12">
        <h1 class="text-4xl font-bold text-gray-900 mb-4">How-To Guides</h1>
        <p class="text-xl text-gray-600 max-w-3xl mx-auto">
            Step-by-step tutorials to help you get the most out of velocity.report
        </p>
    </div>

    <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
        {% for guide in collections.guides | reverse %}
        {% if guide.url != page.url %}
        <a href="{{ guide.url }}" class="feature-card group">
            <h3 class="text-xl font-semibold text-gray-900 mb-2 group-hover:text-blue-600 transition-colors">
                {{ guide.data.title }}
            </h3>
            {% if guide.data.description %}
            <p class="text-gray-600 mb-4">{{ guide.data.description }}</p>
            {% endif %}
            <div class="flex items-center text-sm text-gray-500">
                {% if guide.data.content %}
                <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                </svg>
                <span>{{ guide.template.frontMatter.content | readingTime }} min read</span>
                {% endif %}
            </div>
        </a>
        {% endif %}
        {% endfor %}
    </div>
</div>
