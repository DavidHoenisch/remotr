import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const themeCss = readFileSync(new URL("./theme.css", import.meta.url), "utf8");

const requiredTokens = [
  "--font-sans",
  "--font-mono",
  "--color-canvas",
  "--color-surface",
  "--color-navigation",
  "--color-ink",
  "--color-ink-muted",
  "--color-border",
  "--color-primary",
  "--color-focus",
  "--color-compliant",
  "--color-drifted",
  "--color-error",
  "--space-1",
  "--space-6",
  "--radius-control",
  "--border-hairline",
  "--shadow-overlay",
  "--motion-fast",
  "--navigation-width",
  "--window-min-width",
  "--window-min-height",
];

describe("desktop visual theme contract", () => {
  it("defines the quiet operations-console token vocabulary", () => {
    for (const token of requiredTokens) {
      expect(themeCss, `${token} should be defined`).toMatch(
        new RegExp(`${token}\\s*:`),
      );
    }

    expect(themeCss).toContain("--navigation-width: 224px");
    expect(themeCss).toContain("--window-min-width: 1100px");
    expect(themeCss).toContain("--window-min-height: 720px");
    expect(themeCss).toContain("font-variant-numeric: tabular-nums");
  });

  it("bundles typography and keeps release styling local and restrained", () => {
    expect(themeCss).toContain('@fontsource/ibm-plex-sans/400.css');
    expect(themeCss).toContain('@fontsource/ibm-plex-mono/500.css');
    expect(themeCss).not.toMatch(/url\(["']?https?:/i);
    expect(themeCss).not.toMatch(/(?:linear|radial|conic)-gradient\(/i);
  });

  it("provides visible focus and a reduced-motion override", () => {
    expect(themeCss).toMatch(/:focus-visible\s*\{/);
    expect(themeCss).toMatch(/outline:\s*[^;]*var\(--color-focus\)/);
    expect(themeCss).toMatch(
      /@media\s*\(prefers-reduced-motion:\s*reduce\)/,
    );
    expect(themeCss).toContain("--motion-fast: 1ms");
  });
});
