import { describe, it, expect } from "vitest";
import { theme, statusColor } from "./theme";

describe("statusColor", () => {
  it("maps each status to a palette color", () => {
    expect(statusColor("ok")).toBe(theme.green);
    expect(statusColor("timeout")).toBe(theme.red);
    expect(statusColor("error")).toBe(theme.red);
    expect(statusColor("unknown")).toBe(theme.textDim);
    expect(statusColor("something-else")).toBe(theme.textDim);
  });
});
