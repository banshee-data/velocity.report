# Homepage Responsive GIF Strategies

- **Context:** The VelocityVisualiser section on the homepage includes an animated GIF demonstrating the macOS app. The source recording is a macOS window capture at approximately 3:2 landscape aspect ratio (width:height). This document evaluates strategies for displaying the GIF responsively across viewport sizes.

- **Chosen Approach:** Simple Responsive `<img>` ✅

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

**How it works:**

- `w-full` makes the image fill its container on small screens
- `max-w-3xl` (48rem / 768px) caps the width on larger viewports so the GIF
  doesn't stretch beyond a comfortable viewing size
- `h-auto` preserves the intrinsic aspect ratio at every width
- `loading="lazy"` defers the download until the image is near the viewport,
  improving initial page load
- `rounded-xl shadow-lg` provides visual polish consistent with the rest of the
  page

**Pros:** Zero JavaScript, works in all browsers, no build step, simple markup.

**Cons:** The full-resolution GIF is downloaded regardless of device. Acceptable
when the GIF is optimised to a reasonable file size (aim for < 2 MB).

---

## Alternative Approaches

### Option B: Convert GIF to `<video>` (MP4/WebM)

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
  <img src="/img/visualiser-demo.gif" alt="Fallback GIF" />
</video>
```

**Pros:**

- 10–20× smaller file size than GIF at equivalent quality
- Hardware-decoded on all modern devices
- `poster` attribute prevents layout shift and shows a frame before playback

**Cons:**

- Requires a conversion step (e.g. `ffmpeg -i demo.gif -c:v libvpx-vp9 demo.webm`)
- Two source files to maintain (WebM + MP4)
- Slightly more complex markup

**When to adopt:** If the GIF exceeds ~3 MB, converting to video is strongly
recommended. This is the single best optimisation available.

### ~~Option C: `<picture>` with Multiple GIF Sizes~~

```html
<picture>
  <source media="(max-width: 640px)" srcset="/img/visualiser-demo-480w.gif" />
  <source media="(max-width: 1024px)" srcset="/img/visualiser-demo-768w.gif" />
  <img src="/img/visualiser-demo-1200w.gif" alt="..." class="w-full ..." />
</picture>
```

**Why rejected:** Resizing animated GIFs requires specialised tooling
(`gifsicle --resize`), produces multiple large files, and the quality degrades
at smaller sizes because GIF uses palette-based colour. The complexity is not
justified when a single responsive `<img>` works well.

### ~~Option D: CSS `background-image` with Media Queries~~

```css
.visualiser-demo {
  background-image: url("/img/visualiser-demo.gif");
  background-size: contain;
  aspect-ratio: 3 / 2;
}
@media (max-width: 640px) {
  .visualiser-demo {
    background-image: url("/img/visualiser-demo-sm.gif");
  }
}
```

**Why rejected:** Background images have no `alt` text (accessibility concern),
no native lazy loading, and are harder to maintain. Not suitable for primary
content images.

### ~~Option E: Intersection Observer with Static Poster~~

```html
<img
  src="/img/visualiser-demo-poster.webp"
  data-gif="/img/visualiser-demo.gif"
  alt="..."
  class="w-full max-w-3xl ..."
/>
<script>
  const observer = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      if (e.isIntersecting) {
        e.target.src = e.target.dataset.gif;
        observer.unobserve(e.target);
      }
    });
  });
  observer.observe(document.querySelector("[data-gif]"));
</script>
```

**Why rejected:** `loading="lazy"` achieves similar deferred loading natively
without JavaScript. The poster-swap approach adds complexity and a visible
content shift when the GIF starts loading.

---

## GIF Optimisation Checklist

When the macOS window capture GIF is recorded, optimise before committing:

1. **Capture dimensions:** Record at native resolution, then resize to max
   1536px wide (`max-w-3xl` is 768 CSS px; 1536px covers 2× retina displays)
2. **Frame rate:** 10–15 fps is sufficient for UI demos (reduces file size
   significantly vs 30 fps)
3. **Colour palette:** Use `gifsicle --optimize=3 --colors=128` to reduce
   palette
4. **Duration:** Keep the loop under 10 seconds to limit file size
5. **Target size:** Under 2 MB for the GIF; if larger, convert to video
   (Option B)

```bash
# Example optimisation pipeline
gifsicle --optimize=3 --colors=128 --resize-width 1536 \
    input.gif -o visualiser-demo.gif
```

## Accessibility Considerations

- The `alt` attribute provides a text description of the GIF content
- `loading="lazy"` respects bandwidth by deferring off-screen loads
- Users with `prefers-reduced-motion: reduce` may prefer a static image;
  a future enhancement could use a `<picture>` element to swap in a static
  screenshot via a CSS media query on the container

## File Placement

- GIF asset: `public_html/src/images/visualiser-demo.gif`
- Served at: `/img/visualiser-demo.gif` (via Eleventy passthrough copy)
- Placeholder: `public_html/src/images/visualiser-demo-placeholder.svg`
  (remove once GIF is added)
