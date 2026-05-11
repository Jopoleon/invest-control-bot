const TELEGRAM_API_BASE_URL = "https://api.telegram.org";
const WEBHOOK_PATH = "/telegram/webhook";
const UPSTREAM_TIMEOUT_MS = 8000;

function json(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

function telegramError(status, error, description, extra = {}) {
  return json(status, {
    ok: false,
    error_code: status,
    description,
    error,
    ...extra,
  });
}

function extractBotPath(pathname) {
  const match = pathname.match(/^\/bot([^/]+)(\/.*)?$/);
  if (!match) {
    return null;
  }
  return {
    token: match[1],
    suffix: match[2] || "",
  };
}

function configuredSecret(env, name) {
  return String(env[name] || "").trim();
}

function cloneProxyHeaders(request) {
  const headers = new Headers(request.headers);
  headers.delete("host");
  headers.delete("cf-connecting-ip");
  headers.delete("x-forwarded-for");
  headers.delete("x-real-ip");
  return headers;
}

async function proxyTelegramBotAPI(request, env, url) {
  const parsed = extractBotPath(url.pathname);
  if (!parsed) {
    return null;
  }

  const expectedToken = configuredSecret(env, "TELEGRAM_BOT_TOKEN");
  if (!expectedToken) {
    return telegramError(
      500,
      "relay_secret_not_configured",
      "Telegram relay secret TELEGRAM_BOT_TOKEN is not configured",
    );
  }

  // Do not operate as a generic public relay. We only proxy requests that
  // target the bot token explicitly configured for this worker.
  if (parsed.token !== expectedToken) {
    return telegramError(
      403,
      "forbidden",
      "Telegram relay rejected request for a different bot token",
    );
  }

  const upstreamURL = new URL(
    `${TELEGRAM_API_BASE_URL}/bot${expectedToken}${parsed.suffix}${url.search}`,
  );

  return fetchWith502(
    upstreamURL,
    {
      method: request.method,
      headers: cloneProxyHeaders(request),
      body:
        request.method === "GET" || request.method === "HEAD"
          ? undefined
          : request.body,
      redirect: "manual",
    },
    "telegram",
  );
}

async function proxyTelegramWebhook(request, env) {
  if (request.method !== "POST") {
    return json(405, { ok: false, error: "method_not_allowed" });
  }

  const expectedSecret = configuredSecret(env, "TELEGRAM_WEBHOOK_SECRET");
  if (!expectedSecret) {
    return json(500, {
      ok: false,
      error: "webhook_secret_not_configured",
    });
  }

  const receivedSecret = request.headers.get(
    "x-telegram-bot-api-secret-token",
  );
  if (receivedSecret !== expectedSecret) {
    return json(401, { ok: false, error: "unauthorized" });
  }

  const originURL = configuredSecret(env, "TELEGRAM_WEBHOOK_ORIGIN_URL");
  if (!originURL) {
    return json(500, {
      ok: false,
      error: "webhook_origin_url_not_configured",
    });
  }

  const headers = cloneProxyHeaders(request);
  headers.set("x-telegram-bot-api-secret-token", expectedSecret);

  return fetchWith502(new URL(originURL), {
    method: "POST",
    headers,
    body: request.body,
    redirect: "manual",
  });
}

async function fetchWith502(url, init, responseShape = "generic") {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort("upstream_timeout"), UPSTREAM_TIMEOUT_MS);
  try {
    const upstreamResponse = await fetch(url, {
      ...init,
      signal: controller.signal,
    });
    const responseHeaders = new Headers(upstreamResponse.headers);
    responseHeaders.delete("content-encoding");
    responseHeaders.delete("content-length");

    return new Response(upstreamResponse.body, {
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    if (isAbortError(error)) {
      if (responseShape === "telegram") {
        return telegramError(
          502,
          "upstream_timeout",
          `Telegram relay upstream timeout after ${UPSTREAM_TIMEOUT_MS}ms`,
          { timeout_ms: UPSTREAM_TIMEOUT_MS },
        );
      }
      return json(502, {
        ok: false,
        error: "upstream_timeout",
        timeout_ms: UPSTREAM_TIMEOUT_MS,
      });
    }
    const message = error instanceof Error ? error.message : String(error);
    if (responseShape === "telegram") {
      return telegramError(
        502,
        "upstream_fetch_failed",
        `Telegram relay upstream fetch failed: ${message}`,
      );
    }
    return json(502, {
      ok: false,
      error: "upstream_fetch_failed",
      message,
    });
  } finally {
    clearTimeout(timeout);
  }
}

function isAbortError(error) {
  return (
    error instanceof DOMException && error.name === "AbortError"
  ) || (
    error instanceof Error && error.name === "AbortError"
  ) || (
    String(error) === "upstream_timeout"
  );
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === WEBHOOK_PATH) {
      return proxyTelegramWebhook(request, env);
    }

    const botAPIResponse = await proxyTelegramBotAPI(request, env, url);
    if (botAPIResponse) {
      return botAPIResponse;
    }

    return json(404, { ok: false, error: "not_found" });
  },
};
