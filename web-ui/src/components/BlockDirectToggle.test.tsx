import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import BlockDirectToggle from "./BlockDirectToggle";

interface Rule {
  match: { type: string; value: string };
  action: string;
  enabled: boolean;
}

// Fake proxy-core: serves /api/block-direct (GET + PUT) and /api/plugins.
function stubApi(state: { enabled: boolean; rules?: Rule[] }) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/api/plugins")) {
      return Response.json({ rules: state.rules ?? [] });
    }
    if (init?.method === "PUT") {
      state.enabled = JSON.parse(String(init.body)).enabled;
    }
    return Response.json({ enabled: state.enabled });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("BlockDirectToggle", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stays disabled until the kill-switch state has loaded", async () => {
    stubApi({ enabled: false });
    render(<BlockDirectToggle />);

    const toggle = screen.getByRole("switch");
    expect(toggle).toBeDisabled();
    await waitFor(() => expect(toggle).toBeEnabled());
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText(/falls back to a direct/)).toBeInTheDocument();
  });

  it("PUTs the new state and reflects the server's answer", async () => {
    const fetchMock = stubApi({ enabled: false });
    render(<BlockDirectToggle />);
    const toggle = screen.getByRole("switch");
    await waitFor(() => expect(toggle).toBeEnabled());

    fireEvent.click(toggle);

    await waitFor(() => expect(toggle).toHaveAttribute("aria-checked", "true"));
    await waitFor(() => expect(toggle).toBeEnabled());
    expect(screen.getByText(/kill-switch active/)).toBeInTheDocument();
    const put = fetchMock.mock.calls.find(([, init]) => init?.method === "PUT");
    expect(put?.[0]).toMatch(/\/api\/block-direct$/);
    expect(JSON.parse(String(put?.[1]?.body))).toEqual({ enabled: true });
  });

  it("reverts the optimistic flip when the PUT fails", async () => {
    const fetchMock = stubApi({ enabled: false });
    render(<BlockDirectToggle />);
    const toggle = screen.getByRole("switch");
    await waitFor(() => expect(toggle).toBeEnabled());

    fetchMock.mockRejectedValueOnce(new Error("network down"));
    fireEvent.click(toggle);

    await waitFor(() => expect(toggle).toBeEnabled());
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });

  it("names enabled direct rules that still bypass the kill-switch", async () => {
    stubApi({
      enabled: true,
      rules: [
        { match: { type: "domain", value: "lan.example" }, action: "direct", enabled: true },
        { match: { type: "domain", value: "off.example" }, action: "direct", enabled: false },
        { match: { type: "port", value: "25" }, action: "block", enabled: true },
      ],
    });
    render(<BlockDirectToggle />);

    const warning = await screen.findByText(/heads-up/);
    expect(warning.parentElement).toHaveTextContent("1 enabled direct rule still send");
    expect(warning.parentElement).toHaveTextContent("domain:lan.example");
    expect(warning.parentElement).not.toHaveTextContent("off.example");
  });

  it("shows no direct-rule warning while the kill-switch is off", async () => {
    stubApi({
      enabled: false,
      rules: [{ match: { type: "domain", value: "lan.example" }, action: "direct", enabled: true }],
    });
    render(<BlockDirectToggle />);

    await waitFor(() => expect(screen.getByRole("switch")).toBeEnabled());
    expect(screen.queryByText(/heads-up/)).not.toBeInTheDocument();
  });
});
