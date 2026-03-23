# Homepage — Responsive GIF Strategy

Active plan: [homepage-responsive-gif-strategies.md](../plans/homepage-responsive-gif-strategies.md)

## Chosen Approach: Simple Responsive `<img>`

```html
<div class="mx-auto max-w-3xl mb-8">
  <img
    src="/img/visualiser-demo.gif"
    alt="VelocityVisualiser showing real-time LiDAR track visualisation"
    class="w-full h-auto rounded-xl shadow-lg"
    loading="lazy"
    width="900"
    height="600"
  />
</div>
```

- `w-full` fills container on small screens.
- `max-w-3xl` (768px) caps width on larger viewports.
- `h-auto` preserves intrinsic aspect ratio.
- `loading="lazy"` defers download until near viewport.
- Zero JavaScript, all browsers, no build step.

## GIF Optimisation Checklist

1. Resize to max 1536px wide (2× retina for `max-w-3xl`).
2. Frame rate: 10–15 fps (sufficient for UI demos).
3. `gifsicle --optimize=3 --colors=128`.
4. Duration: under 10 seconds.
5. Target size: under 2 MB. If larger, convert to video.

## Upgrade Path: Video

If the GIF exceeds ~3 MB, convert to MP4/WebM:

```html
<video
  autoplay
  loop
  muted
  playsinline
  class="w-full max-w-3xl mx-auto rounded-xl shadow-lg"
  poster="/img/visualiser-demo-poster.webp"
>
  <source src="/img/visualiser-demo.webm" type="video/webm" />
  <source src="/img/visualiser-demo.mp4" type="video/mp4" />
</video>
```

10–20× smaller than GIF at equivalent quality.

## File Placement

- GIF asset: `public_html/src/images/visualiser-demo.gif`
- Served at: `/img/visualiser-demo.gif` (Eleventy passthrough copy)
- Placeholder: `public_html/src/images/visualiser-demo-placeholder.svg`
