import { describe, it, expect } from "vitest";
import { displayEndpointName, displayEndpointId, displayProtocol } from "./display";

describe("displayEndpointName", () => {
  it("strips MoaV- and trailing source, pretty-cases", () => {
    expect(displayEndpointName("MoaV-Hysteria2-beezhantest2", undefined, "beezhantest2")).toBe("Hysteria2");
  });
  it("strips a leading source and pretty-cases wireguard", () => {
    expect(displayEndpointName("beezhantest2/wireguard", undefined, "beezhantest2")).toBe("WireGuard");
  });
  it("strips the sidecar- prefix", () => {
    expect(displayEndpointName("sidecar-masterdns")).toBe("MasterDNS");
  });
  it("falls back to id when name is empty", () => {
    expect(displayEndpointName("", "epid")).toBe("epid");
  });
  it("passes unknown labels through unchanged", () => {
    expect(displayEndpointName("MyCustomThing")).toBe("MyCustomThing");
  });
});

describe("displayEndpointId", () => {
  it("strips only the leading sidecar: tag", () => {
    expect(displayEndpointId("sidecar:psiphon")).toBe("psiphon");
    expect(displayEndpointId("vless:1.2.3.4:443")).toBe("vless:1.2.3.4:443");
  });
});

describe("displayProtocol", () => {
  it("prefers the sidecar kind when present", () => {
    expect(displayProtocol("sidecar", "masterdns")).toBe("masterdns");
    expect(displayProtocol("sidecar")).toBe("sidecar");
    expect(displayProtocol("vless")).toBe("vless");
  });
});
