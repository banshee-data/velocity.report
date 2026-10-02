// /scenes/ was renamed /surveys/; leave a redirect at each old published URL.
const escapeAttr = (value) => String(value).replace(/&/g, "&amp;").replace(/"/g, "&quot;");

class SceneRedirects {
  data() {
    return {
      layout: false,
      eleventyExcludeFromCollections: true,
      pagination: {
        data: "scenes.sites",
        size: 1,
        alias: "site",
        before: (sites) => [{ id: "" }, ...sites.filter((site) => site.published)],
      },
      permalink: (data) => `/scenes/${data.site.id ? `${data.site.id}/` : ""}index.html`,
    };
  }

  render(data) {
    const path = `/surveys/${data.site.id ? `${data.site.id}/` : ""}`;
    // The build's HTML transform prefixes href="/..." itself; meta content it does not touch.
    const href = escapeAttr(path);
    const refresh = escapeAttr(this.url(path));
    return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Scenes are now surveys</title>
<meta name="robots" content="noindex">
<link rel="canonical" href="${href}">
<meta http-equiv="refresh" content="0; url=${refresh}">
</head>
<body>
<p>Scenes are now called surveys. <a href="${href}">Continue to the survey page</a>.</p>
</body>
</html>
`;
  }
}

module.exports = SceneRedirects;
