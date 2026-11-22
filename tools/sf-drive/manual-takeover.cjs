async function confirmManualTakeover(page) {
  if ((await page.locator("#insuranceMode").textContent()) === "Meat mode")
    return;
  await page.keyboard.press("ArrowUp");
  const dialog = page.getByRole("dialog", { name: "Switch to Meat mode?" });
  await dialog.getByRole("button", { name: "Take manual control" }).click();
  await page.waitForFunction(
    () => document.querySelector("#insuranceMode").textContent === "Meat mode",
  );
}

module.exports = { confirmManualTakeover };
