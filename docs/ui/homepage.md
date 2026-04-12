# Homepage: responsive GIF strategy

Records the chosen strategy for displaying the LiDAR visualiser demo GIF on the public homepage in a responsive, performant way across device sizes.

## Chosen approach: simple responsive `<img>`

A `<div>` wrapper with `mx-auto max-w-3xl mb-8` centres the content and
caps the width. Inside, an `<img>` tag with:

- `src="/img/visualiser-demo.gif"`
- `alt="VelocityVisualiser showing real-time LiDAR track visualisation"`
- `class="w-full h-auto rounded-xl shadow-lg"`
- `loading="lazy"` to defer download until near viewport
- `width="900"` and `height="600"` for intrinsic sizing

Key properties: `w-full` fills container on small screens.

- `max-w-3xl` (768px) caps width on larger viewports.
- `h-auto` preserves intrinsic aspect ratio.
- `loading="lazy"` defers download until near viewport.
- Zero JavaScript, all browsers, no build step.

## GIF optimisation checklist

1. Resize to max 1536px wide (2× retina for `max-w-3xl`).
2. Frame rate: 10–15 fps (sufficient for UI demos).
3. `gifsicle --optimize=3 --colors=128`.
4. Duration: under 10 seconds.
5. Target size: under 2 MB. If larger, convert to video.

## Upgrade path: video

If the GIF exceeds ~3 MB, convert to MP4/WebM and use a `<video>` element
with `autoplay`, `loop`, `muted`, and `playsinline` attributes. Apply the
same Tailwind classes (`w-full max-w-3xl mx-auto rounded-xl shadow-lg`).
Set `poster="/img/visualiser-demo-poster.webp"` for a static placeholder.
Include two `<source>` elements: WebM (`/img/visualiser-demo.webm`) as
primary and MP4 (`/img/visualiser-demo.mp4`) as fallback.

10–20× smaller than GIF at equivalent quality.

## File placement

- GIF asset: `public_html/src/images/visualiser-demo.gif`
- Served at: `/img/visualiser-demo.gif` (Eleventy passthrough copy)
- Placeholder: `public_html/src/images/visualiser-demo-placeholder.svg`
