import { expect, test } from "bun:test";

import { buildFrontendTokenURL, frontendSessionCookieName, handleFrontendTokenRequest, resolveFrontendAccessToken, resolveFrontendHost, resolveFrontendPort, validateFrontendApiRequest } from "./frontendAuth";

test("frontend host and port default to loopback", () => {
  expect(resolveFrontendHost({})).toBe("127.0.0.1");
  expect(resolveFrontendPort({})).toBe(5020);
  expect(resolveFrontendPort({ MINICLAW_FRONTEND_PORT: "5080" })).toBe(5080);
  expect(resolveFrontendPort({ MINICLAW_FRONTEND_PORT: "bad" })).toBe(5020);
});

test("frontend access token uses configured value when present", () => {
  expect(resolveFrontendAccessToken({ MINICLAW_FRONTEND_TOKEN: "fixed-token" })).toEqual({ token: "fixed-token", generated: false });
  const generated = resolveFrontendAccessToken({});
  expect(generated.generated).toBe(true);
  expect(generated.token.length).toBeGreaterThan(20);
});

test("buildFrontendTokenURL appends the session token", () => {
  expect(buildFrontendTokenURL("http://127.0.0.1:5020/", "secret")).toBe("http://127.0.0.1:5020/?token=secret");
});

test("handleFrontendTokenRequest sets an http-only session cookie", () => {
  const response = handleFrontendTokenRequest(new Request("http://127.0.0.1:5020/?token=secret"), "secret");
  expect(response?.status).toBe(303);
  expect(response?.headers.get("Location")).toBe("http://127.0.0.1:5020/");
  const cookie = response?.headers.get("Set-Cookie") || "";
  expect(cookie).toContain(`${frontendSessionCookieName}=secret`);
  expect(cookie).toContain("HttpOnly");
  expect(cookie).toContain("SameSite=Strict");
});

test("validateFrontendApiRequest requires a session token", () => {
  const result = validateFrontendApiRequest(new Request("http://127.0.0.1:5020/api/health"), "secret");
  expect(result.ok).toBe(false);
  expect(result.status).toBe(401);
});

test("validateFrontendApiRequest accepts cookie-backed same-origin reads", () => {
  const request = new Request("http://127.0.0.1:5020/api/health", {
    headers: { Cookie: `${frontendSessionCookieName}=secret` },
  });
  expect(validateFrontendApiRequest(request, "secret").ok).toBe(true);
});

test("validateFrontendApiRequest rejects cookie-backed cross-origin writes", () => {
  const request = new Request("http://127.0.0.1:5020/api/tasks", {
    method: "POST",
    headers: {
      Cookie: `${frontendSessionCookieName}=secret`,
      Origin: "http://evil.example",
    },
  });
  const result = validateFrontendApiRequest(request, "secret");
  expect(result.ok).toBe(false);
  expect(result.status).toBe(403);
});

test("validateFrontendApiRequest accepts cookie-backed same-origin writes", () => {
  const request = new Request("http://127.0.0.1:5020/api/tasks", {
    method: "POST",
    headers: {
      Cookie: `${frontendSessionCookieName}=secret`,
      Origin: "http://127.0.0.1:5020",
    },
  });
  expect(validateFrontendApiRequest(request, "secret").ok).toBe(true);
});
