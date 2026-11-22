module.exports = () => {
  const address = process.env.SF_CITY_ROADS_MANIFEST;
  if (!address) return "";
  const url = new URL(address);
  if (
    url.protocol !== "https:" ||
    url.username ||
    url.password ||
    !url.pathname.endsWith("/manifest.json")
  )
    throw new Error(
      "SF_CITY_ROADS_MANIFEST must be an HTTPS manifest URL without credentials.",
    );
  return url.href;
};
