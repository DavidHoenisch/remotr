import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const themeCss = readFileSync(new URL("./theme.css", import.meta.url), "utf8");

function colorToken(name) {
  const value = themeCss.match(
    new RegExp(`${name}\\s*:\\s*(#[0-9a-f]{6})`, "i"),
  )?.[1];
  expect(value, `${name} should be a six-digit color token`).toBeDefined();
  return value;
}

function relativeLuminance(hex) {
  const channels = hex
    .slice(1)
    .match(/../g)
    .map((value) => Number.parseInt(value, 16) / 255)
    .map((value) =>
      value <= 0.04045
        ? value / 12.92
        : ((value + 0.055) / 1.055) ** 2.4,
    );
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrastRatio(foreground, background) {
  const values = [foreground, background]
    .map(relativeLuminance)
    .toSorted((left, right) => right - left);
  return (values[0] + 0.05) / (values[1] + 0.05);
}

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
    expect(themeCss).toMatch(/scroll-behavior:\s*auto\s*!important/);
    expect(themeCss).toMatch(/animation-duration:\s*1ms\s*!important/);
    expect(themeCss).toMatch(/transition-duration:\s*1ms\s*!important/);
  });

  it("meets the 4.5 to 1 contrast target for interface and status text", () => {
    const pairs = [
      ["--color-ink", "--color-canvas"],
      ["--color-ink-muted", "--color-surface"],
      ["--color-surface-raised", "--color-primary"],
      ["--color-compliant", "--color-compliant-soft"],
      ["--color-drifted", "--color-drifted-soft"],
      ["--color-error", "--color-error-soft"],
      ["--color-info", "--color-info-soft"],
      ["--color-neutral", "--color-neutral-soft"],
    ];

    for (const [foreground, background] of pairs) {
      expect(
        contrastRatio(colorToken(foreground), colorToken(background)),
        `${foreground} on ${background}`,
      ).toBeGreaterThanOrEqual(4.5);
    }
  });
});
