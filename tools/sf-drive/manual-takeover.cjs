async function confirmManualTakeover(page) {
  if ((await page.locator("#insuranceMode").textContent()) === "Meat mode")
    return;
  const dialog = page.getByRole("dialog", { name: "Switch to Meat mode?" });
  await page.keyboard.down("ArrowUp");
  try {
    await dialog.waitFor();
  } finally {
    await page.keyboard.up("ArrowUp");
  }
  await dialog.getByRole("button", { name: "Take manual control" }).click();
  await page.waitForFunction(
    () => document.querySelector("#insuranceMode").textContent === "Meat mode",
  );
}

module.exports = { confirmManualTakeover };
