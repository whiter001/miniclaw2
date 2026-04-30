import { randomBytes, timingSafeEqual } from "node:crypto";

export const frontendSessionCookieName = "miniclaw_frontend_token";
const defaultFrontendPort = 5020;
const sessionMaxAgeSeconds = 12 * 60 * 60;
const tokenQueryParam = "token";

export interface FrontendAccessToken {
  token: string;
  generated: boolean;
}

export interface FrontendAuthResult {
  ok: boolean;
  status: number;
  message: string;
}

export function resolveFrontendHost(env: NodeJS.ProcessEnv = process.env) {
  return env.MINICLAW_FRONTEND_HOST?.trim() || "127.0.0.1";
}

export function resolveFrontendPort(env: NodeJS.ProcessEnv = process.env) {
  const raw = env.MINICLAW_FRONTEND_PORT?.trim() || env.PORT?.trim() || String(defaultFrontendPort);
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed) || parsed <= 0 || parsed > 65535) {
    return defaultFrontendPort;
  }
  return parsed;
}

export function resolveFrontendAccessToken(env: NodeJS.ProcessEnv = process.env): FrontendAccessToken {
  const configured = env.MINICLAW_FRONTEND_TOKEN?.trim();
  if (configured) {
    return { token: configured, generated: false };
  }
  return { token: randomBytes(32).toString("base64url"), generated: true };
}

export function buildFrontendTokenURL(serverURL: string, token: string) {
  const url = new URL(serverURL);
  url.searchParams.set(tokenQueryParam, token);
  return url.toString();
}

export function handleFrontendTokenRequest(request: Request, expectedToken: string): Response | null {
  const url = new URL(request.url);
  const providedToken = url.searchParams.get(tokenQueryParam);
  if (providedToken === null) {
    return null;
  }
  if (!tokenMatches(providedToken, expectedToken)) {
    return Response.json({ error: "invalid frontend access token" }, { status: 401 });
  }
  url.searchParams.delete(tokenQueryParam);
  const response = Response.redirect(url.toString(), 303);
  response.headers.append("Set-Cookie", buildSessionCookie(expectedToken));
  return response;
}

export function validateFrontendApiRequest(request: Request, expectedToken: string): FrontendAuthResult {
  if (tokenMatches(headerToken(request), expectedToken)) {
    return allow();
  }
  if (!tokenMatches(cookieToken(request), expectedToken)) {
    return { ok: false, status: 401, message: "frontend access token is required" };
  }
  if (isMutatingMethod(request.method) && !isSameOriginRequest(request)) {
    return { ok: false, status: 403, message: "same-origin request is required" };
  }
  return allow();
}

function buildSessionCookie(token: string) {
  return [
    `${frontendSessionCookieName}=${encodeURIComponent(token)}`,
    "Path=/",
    "HttpOnly",
    "SameSite=Strict",
    `Max-Age=${sessionMaxAgeSeconds}`,
  ].join("; ");
}

function headerToken(request: Request) {
  const direct = request.headers.get("X-MiniClaw-Frontend-Token")?.trim();
  if (direct) {
    return direct;
  }
  const authorization = request.headers.get("Authorization")?.trim() || "";
  const prefix = "Bearer ";
  if (authorization.toLowerCase().startsWith(prefix.toLowerCase())) {
    return authorization.slice(prefix.length).trim();
  }
  return "";
}

function cookieToken(request: Request) {
  const cookieHeader = request.headers.get("Cookie") || "";
  for (const part of cookieHeader.split(";")) {
    const separator = part.indexOf("=");
    if (separator < 0) {
      continue;
    }
    const key = part.slice(0, separator).trim();
    if (key !== frontendSessionCookieName) {
      continue;
    }
    return decodeURIComponent(part.slice(separator + 1).trim());
  }
  return "";
}

function isSameOriginRequest(request: Request) {
  const expectedOrigin = new URL(request.url).origin;
  const origin = request.headers.get("Origin")?.trim();
  if (origin) {
    return origin === expectedOrigin;
  }
  const referer = request.headers.get("Referer")?.trim();
  if (!referer) {
    return false;
  }
  try {
    return new URL(referer).origin === expectedOrigin;
  } catch {
    return false;
  }
}

function isMutatingMethod(method: string) {
  return !["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase());
}

function allow(): FrontendAuthResult {
  return { ok: true, status: 200, message: "ok" };
}

function tokenMatches(candidate: string | undefined, expected: string) {
  if (!candidate || !expected) {
    return false;
  }
  const candidateBuffer = Buffer.from(candidate);
  const expectedBuffer = Buffer.from(expected);
  if (candidateBuffer.length !== expectedBuffer.length) {
    return false;
  }
  return timingSafeEqual(candidateBuffer, expectedBuffer);
}
