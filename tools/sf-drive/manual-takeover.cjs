async function confirmManualTakeover(page) {
  const manualMode = page.locator('#insuranceHud[data-mode="manual"]');
  if ((await manualMode.count()) > 0) return;
  const dialog = page.locator("#takeoverDialog");
  await page.keyboard.down("ArrowUp");
  try {
    await dialog.waitFor();
  } finally {
    await page.keyboard.up("ArrowUp");
  }
  await dialog.getByRole("button", { name: "Take manual control" }).click();
  await manualMode.waitFor();
}

module.exports = { confirmManualTakeover };
