// One archive page per published survey with 3D playback.
//
// Playback fetches frames as it reaches them, so saving a survey page to a web
// archive keeps only the first two. The archive's outlink capture does not fill
// the gap: one save followed exactly 20 of a page's links, and a survey has more
// files than that. A save does record whatever the page fetches while it is
// open, so this page fetches every file, and the survey page stays light.
class SurveyArchivePages {
  data() {
    return {
      layout: "survey-archive.njk",
      noindex: true,
      pagination: {
        data: "surveys.sites",
        size: 1,
        alias: "survey",
        before: (sites) =>
          sites.filter(
            (site) =>
              site.published && site.recordings.some((r) => r.hasPlayback),
          ),
      },
      permalink: (data) => `/surveys/${data.survey.id}/archive/index.html`,
      eleventyComputed: {
        title: (data) => `Archive copy: ${data.survey.title} — velocity.report`,
      },
    };
  }

  render() {
    return "";
  }
}

module.exports = SurveyArchivePages;
