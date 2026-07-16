import { expect, test, type Page } from "@playwright/test";

const viewports = [
  { height: 900, label: "1440x900", width: 1440 },
  { height: 720, label: "1100x720", width: 1100 },
] as const;

async function openFixture(page: Page, state: string) {
  await page.goto(`/visual.html?state=${state}`);
  await page.evaluate(() => document.fonts.ready);
  await expect(page.locator(".app-shell")).toBeVisible();
}

for (const viewport of viewports) {
  test.describe(`${viewport.label} desktop visual evidence`, () => {
    test.use({ viewport });

    test("populated Endpoint inventory", async ({ page }) => {
      await openFixture(page, "inventory");
      await page
        .getByRole("button", { exact: true, name: "Endpoints" })
        .click();

      const table = page.getByRole("table", { name: "Endpoints" });
      await expect(table).toBeVisible();
      await expect(table.locator("tbody tr")).toHaveCount(4);
      await expect(table.getByText("Compliant", { exact: true })).toBeVisible();
      await expect(page).toHaveScreenshot(
        `populated-inventory-${viewport.label}.png`,
      );
    });

    test("partial Overview", async ({ page }) => {
      await openFixture(page, "partial-overview");

      await expect(
        page.getByRole("heading", { level: 1, name: "Overview" }),
      ).toBeVisible();
      await expect(
        page.getByRole("status", {
          name: "Compliance evidence partially available",
        }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "4 total Endpoints" }),
      ).toBeVisible();
      await expect(page).toHaveScreenshot(
        `partial-overview-${viewport.label}.png`,
      );
    });

    test("Endpoint detail", async ({ page }) => {
      await openFixture(page, "endpoint-detail");
      await page
        .getByRole("button", { exact: true, name: "Endpoints" })
        .click();
      await page
        .getByRole("button", { name: "Inspect endpoint-alpha" })
        .click();

      const dialog = page.getByRole("dialog", {
        name: "Endpoint endpoint-alpha",
      });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByRole("tab")).toHaveCount(5);
      await expect(dialog.getByRole("tabpanel")).toHaveAccessibleName(
        "Overview",
      );
      await expect(page).toHaveScreenshot(
        `endpoint-detail-${viewport.label}.png`,
      );
    });

    test("initial connection failure", async ({ page }) => {
      await openFixture(page, "connection-failure");

      const recovery = page.getByRole("region", {
        name: "Connection recovery",
      });
      await expect(recovery).toBeVisible();
      await expect(
        recovery.getByRole("heading", {
          name: "Production connection failed",
        }),
      ).toBeVisible();
      await expect(page.getByRole("table")).toHaveCount(0);
      await expect(page).toHaveScreenshot(
        `connection-failure-${viewport.label}.png`,
      );
    });

    test("destructive typed confirmation", async ({ page }) => {
      await openFixture(page, "destructive-confirmation");

      const dialog = page.getByRole("dialog", {
        name: "Remove Endpoint endpoint-alpha",
      });
      const confirm = dialog.getByRole("button", {
        exact: true,
        name: "Remove Endpoint",
      });
      await expect(dialog).toBeVisible();
      await expect(confirm).toBeDisabled();
      await expect(dialog.getByText("This action cannot be undone.")).toBeVisible();
      await expect(page).toHaveScreenshot(
        `destructive-confirmation-${viewport.label}.png`,
      );

      await dialog
        .getByRole("textbox", { name: "Type endpoint-alpha to confirm" })
        .fill("endpoint-alpha");
      await expect(confirm).toBeEnabled();
    });
  });
}
