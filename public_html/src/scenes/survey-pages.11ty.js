// One page per published survey site, paginated from src/_data/surveys.js.
//
// These used to be an index.njk per site, written by the scene-map generator
// with the site's title, length and angles copied into its front matter. The
// copies were a second record of what scenes.json already held, so they are
// derived here at build time instead: a site gets a page exactly when it has a
// published export, and nothing about it is stored twice.
const exportOf = (site) =>
  site.recordings.find((r) => r.assetsUrl.endsWith("/assets")) ?? null;

const minutesOf = (site) => Math.round((exportOf(site)?.durationSec ?? 0) / 60);
const hasClip = (site) => exportOf(site)?.hasPointCloud === true;

class SurveyPages {
  data() {
    return {
      layout: "scene.njk",
      pagination: {
        data: "surveys.sites",
        size: 1,
        alias: "survey",
        before: (sites) =>
          sites.filter((site) => site.published && exportOf(site)),
        addAllPagesToCollections: true,
      },
      permalink: (data) => `/surveys/${data.survey.id}/index.html`,
      eleventyComputed: {
        sceneId: (data) => data.survey.id,
        sceneName: (data) => data.survey.title,
        title: (data) => `${data.survey.title}: LiDAR survey — velocity.report`,
        description: (data) =>
          hasClip(data.survey)
            ? `${minutesOf(data.survey)} minutes at a San Francisco junction, with a point-cloud view over the opening 30 seconds. Measured by roadside LiDAR, with no camera images or number plates.`
            : `${minutesOf(data.survey)} minutes at a San Francisco junction, measured by roadside LiDAR. Trajectories only, no camera images or number plates.`,
        sceneIntro: (data) =>
          hasClip(data.survey)
            ? `${minutesOf(data.survey)} minutes at this junction, with the point cloud visible over the opening 30 seconds.`
            : `${minutesOf(data.survey)} minutes at this junction, measured by roadside LiDAR.`,
        sceneHasClip: (data) => hasClip(data.survey),
        gridAzimuthDeg: (data) => data.survey.gridAzimuthDeg,
        northAzimuthDeg: (data) => data.survey.northAzimuthDeg,
      },
    };
  }

  render() {
    return "";
  }
}

module.exports = SurveyPages;
