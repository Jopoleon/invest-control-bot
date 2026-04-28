const TELEGRAM_API_BASE_URL = "https://api.telegram.org";
const WEBHOOK_PATH = "/telegram/webhook";

function json(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
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
    return json(500, {
      ok: false,
      error: "relay_secret_not_configured",
    });
  }

  // Do not operate as a generic public relay. We only proxy requests that
  // target the bot token explicitly configured for this worker.
  if (parsed.token !== expectedToken) {
    return json(403, {
      ok: false,
      error: "forbidden",
    });
  }

  const upstreamURL = new URL(
    `${TELEGRAM_API_BASE_URL}/bot${expectedToken}${parsed.suffix}${url.search}`,
  );

  return fetchWith502(upstreamURL, {
    method: request.method,
    headers: cloneProxyHeaders(request),
    body:
      request.method === "GET" || request.method === "HEAD"
        ? undefined
        : request.body,
    redirect: "manual",
  });
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

async function fetchWith502(url, init) {
  try {
    const upstreamResponse = await fetch(url, init);
    const responseHeaders = new Headers(upstreamResponse.headers);
    responseHeaders.delete("content-encoding");
    responseHeaders.delete("content-length");

    return new Response(upstreamResponse.body, {
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    return json(502, {
      ok: false,
      error: "upstream_fetch_failed",
      message: error instanceof Error ? error.message : String(error),
    });
  }
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
