const proj4 = require("proj4");

const GEODETIC = "+proj=longlat +ellps=GRS80";
const ECEF = proj4("+proj=geocent +ellps=GRS80", GEODETIC);
// EPSG:7131 NAD83(2011) / San Francisco CS13.
const SF_GRID = proj4(
  GEODETIC,
  "+proj=tmerc +lat_0=37.75 +lon_0=-122.45 +k=1.000007 +x_0=48000 +y_0=24000 +ellps=GRS80 +units=m",
);

/**
 * Match tools/s2-archive/city_frame.py: the origin uses NAVD88 as
 * ellipsoidal height, so the tileset's -32.2 m globe offset is irrelevant.
 */
function cityToDrive(city, driveOrigin) {
  if (city.format !== "gerardus-city-tiles-1")
    throw new Error(`Unsupported survey city frame: ${city.format}`);
  const { lat_deg: lat, lon_deg: lon, navd88_m: height } = city.origin;
  const phi = (lat * Math.PI) / 180;
  const lambda = (lon * Math.PI) / 180;
  const sinLat = Math.sin(phi);
  const cosLat = Math.cos(phi);
  const sinLon = Math.sin(lambda);
  const cosLon = Math.cos(lambda);
  const origin = ECEF.inverse([lon, lat, height]);

  return ([east, north, up]) => {
    const [lon, lat, z] = ECEF.forward([
      origin[0] -
        sinLon * east -
        sinLat * cosLon * north +
        cosLat * cosLon * up,
      origin[1] +
        cosLon * east -
        sinLat * sinLon * north +
        cosLat * sinLon * up,
      origin[2] + cosLat * north + sinLat * up,
    ]);
    const [x, y] = SF_GRID.forward([lon, lat]);
    return [x - driveOrigin[0], y - driveOrigin[1], z];
  };
}

/** Local affine transform sampled around the accepted sensor pose. */
function surveyTransform(project, pose) {
  const rows = pose.slice(0, 3);
  const centre = rows.map((row) => row[3]);
  const offset = project(centre);
  const axes = [0, 1, 2].map((column) => {
    const plus = project(centre.map((v, i) => v + rows[i][column]));
    const minus = project(centre.map((v, i) => v - rows[i][column]));
    return plus.map((v, i) => (v - minus[i]) / 2);
  });
  return offset.map((v, i) => [...axes.map((axis) => axis[i]), v]);
}

function sfSurveyData(surveys, cityIndex, driveManifest) {
  const project = cityToDrive(
    cityIndex.city_frame,
    driveManifest.origin_epsg7131,
  );
  const tiles = new Set(driveManifest.tiles.map((tile) => tile.t));
  return cityIndex.sites.flatMap((site) => {
    const { scene, placement } = site;
    if (!site.published || !scene || !tiles.has(site.cells?.l13)) return [];
    if (placement?.state !== "placed" || !placement.sensor_to_city) return [];

    // The accepted pose belongs to this export. A later visit needs its own
    // placement before it can be drawn in the game.
    const recording = surveys.byId[site.id]?.recordings.find(
      (r) => `${r.assetsUrl}/` === scene.assets && r.hasPlayback && r.speed,
    );
    if (!recording) return [];
    const transform = surveyTransform(project, placement.sensor_to_city);
    const { count, bins, p50, p85, p98, max } = recording.speed;
    return [
      {
        id: site.id,
        title: site.title,
        x: transform[0][3],
        y: transform[1][3],
        transform,
        manifest: `${scene.assets.slice(1)}manifest.json`,
        page: `${scene.page.slice(1)}?recording=${encodeURIComponent(recording.id)}`,
        limit: surveys.byId[site.id].speedLimitMph,
        speed: { count, bins, p50, p85, p98, max },
      },
    ];
  });
}

module.exports = { cityToDrive, sfSurveyData };
