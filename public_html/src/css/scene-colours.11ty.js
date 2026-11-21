module.exports = class {
  data() {
    return {
      permalink: "/css/scene-colours.css",
      eleventyExcludeFromCollections: true,
    };
  }

  async render() {
    const { SCENE_CSS_COLOURS } = await import("../js/scene-colours.js");
    const tokens = Object.entries(SCENE_CSS_COLOURS).map(([name, value]) => {
      const kebab = name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);
      return `  --scene-${kebab}: ${value};`;
    });
    return `:root {\n${tokens.join("\n")}\n}\n`;
  }
};
